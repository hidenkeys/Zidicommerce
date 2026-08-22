package fieldservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) StartMatching(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) error {
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return err
	}
	if request.Status == RequestAssigned || request.Status == RequestCompleted {
		return nil
	}
	settings, err := s.ensureSettings(ctx, actor.OrganizationID)
	if err != nil {
		return err
	}
	ranked, err := s.buildRanking(ctx, actor.OrganizationID, request, settings)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Where("request_id = ?", request.ID).Delete(&Match{}).Error; err != nil {
		return err
	}
	for index, row := range ranked {
		match := Match{
			ID:             uuid.New(),
			OrganizationID: actor.OrganizationID,
			RequestID:      request.ID,
			ProviderID:     mustUUID(row.ProviderID),
			Rank:           index + 1,
			Score:          row.Total,
			Breakdown:      encodeBreakdown(row),
		}
		if err := s.db.WithContext(ctx).Create(&match).Error; err != nil {
			return err
		}
	}
	s.audit(ctx, actor, "service_request", request.ID, "match_generated", fmt.Sprintf(`{"count":%d}`, len(ranked)))
	if len(ranked) == 0 {
		_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, "We couldn't find an available professional right now. A coordinator will follow up shortly.")
		return s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestDispatching, "updated_at": s.now()}).Error
	}
	if err := s.db.WithContext(ctx).Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestDispatching, "updated_at": s.now()}).Error; err != nil {
		return err
	}
	return s.notifyNextProvider(ctx, actor, request.ID)
}

func (s *Service) buildRanking(ctx context.Context, organizationID uuid.UUID, request Request, settings Settings) ([]ScoreBreakdown, error) {
	var providers []Provider
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Preload("Pools").Find(&providers).Error; err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(providers))
	for _, provider := range providers {
		poolIDs := make([]string, 0, len(provider.Pools))
		areas := []string{provider.Area}
		for _, pool := range provider.Pools {
			poolIDs = append(poolIDs, pool.ID.String())
		}
		var active int64
		_ = s.db.WithContext(ctx).Model(&Assignment{}).Where("organization_id = ? AND provider_id = ? AND status NOT IN ?", organizationID, provider.ID, []string{RequestCompleted, RequestCancelled}).Count(&active)
		candidates = append(candidates, Candidate{
			ProviderID:    provider.ID.String(),
			Name:          provider.Name,
			PoolIDs:       poolIDs,
			Availability:  provider.Availability,
			Active:        provider.Status == "active",
			Latitude:      provider.Latitude,
			Longitude:     provider.Longitude,
			Areas:         areas,
			Rating:        provider.RatingAverage,
			JobsCompleted: provider.JobsCompleted,
			ActiveJobs:    int(active),
		})
	}
	weights := MatchWeights{Service: settings.WeightService, Availability: settings.WeightAvailability, Distance: settings.WeightDistance, Rating: settings.WeightRating, Experience: settings.WeightExperience}
	return RankProviders(request.PoolID.String(), CustomerLocation{Latitude: request.Latitude, Longitude: request.Longitude, Area: request.Area}, candidates, weights, settings.MaxDistanceKM), nil
}

func (s *Service) ListMatches(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) ([]Match, error) {
	if err := s.requireOwner(actor); err != nil {
		return nil, err
	}
	var matches []Match
	err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ?", actor.OrganizationID, requestID).Preload("Provider").Order("rank ASC").Find(&matches).Error
	return matches, err
}

func (s *Service) notifyNextProvider(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) error {
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return err
	}
	if request.AssignedProviderID != nil {
		return nil
	}
	settings, err := s.ensureSettings(ctx, actor.OrganizationID)
	if err != nil {
		return err
	}
	var matches []Match
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).Order("rank ASC").Find(&matches).Error; err != nil {
		return err
	}
	tried := map[uuid.UUID]struct{}{}
	var previous []DispatchAttempt
	_ = s.db.WithContext(ctx).Where("request_id = ?", requestID).Find(&previous)
	for _, attempt := range previous {
		if attempt.Status != DispatchCancelled {
			tried[attempt.ProviderID] = struct{}{}
		}
	}
	var next *Match
	for i := range matches {
		if _, ok := tried[matches[i].ProviderID]; ok {
			continue
		}
		next = &matches[i]
		break
	}
	if next == nil {
		_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, "We're still looking for an available professional. Hang tight.")
		return nil
	}
	provider, err := s.getProvider(ctx, actor.OrganizationID, next.ProviderID)
	if err != nil {
		return err
	}
	expires := s.now().Add(time.Duration(settings.AcceptanceWindowSeconds) * time.Second)
	attempt := DispatchAttempt{
		ID:             uuid.New(),
		OrganizationID: actor.OrganizationID,
		RequestID:      request.ID,
		ProviderID:     provider.ID,
		Status:         DispatchNotified,
		NotifiedAt:     s.now(),
		ExpiresAt:      expires,
		Metadata:       "{}",
	}
	if err := s.db.WithContext(ctx).Create(&attempt).Error; err != nil {
		return err
	}
	portal := settings.ProviderPortalBaseURL
	if portal == "" {
		portal = "the provider portal"
	}
	msg := fmt.Sprintf("New service request available.\n\nService: %s\nLocation: %s\nCustomer issue: %s\n\nPlease open your Zidi provider portal to review and accept this job.\n%s", request.Pool.Name, displayLocation(request), clip(request.Description, 140), portal)
	_ = s.notify(ctx, actor.OrganizationID, firstNonEmpty(provider.WhatsAppNumber, provider.Phone), msg)
	s.audit(ctx, actor, "service_request", request.ID, "provider_notified", fmt.Sprintf(`{"provider":%q}`, provider.Name))
	if s.jobs != nil {
		_, _ = s.jobs.Enqueue(ctx, jobs.EnqueueInput{
			OrganizationID: &actor.OrganizationID,
			JobType:        jobs.JobTypeServiceDispatchTimeout,
			Payload:        map[string]any{"attempt_id": attempt.ID.String(), "request_id": request.ID.String()},
			IdempotencyKey: "dispatch-timeout:" + attempt.ID.String(),
			AvailableAt:    &expires,
			MaxAttempts:    1,
		})
	}
	return nil
}

func (s *Service) ProcessDispatchTimeout(ctx context.Context, job jobs.Job) error {
	payload := parseMap(job.Payload)
	attemptID, err := uuid.Parse(stringValue(payload["attempt_id"]))
	if err != nil {
		return jobs.PermanentError{Err: fmt.Errorf("invalid attempt")}
	}
	var attempt DispatchAttempt
	if err := s.db.WithContext(ctx).Where("id = ?", attemptID).First(&attempt).Error; err != nil {
		return nil
	}
	if attempt.Status != DispatchNotified {
		return nil
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&DispatchAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{"status": DispatchTimeout, "responded_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	actor := auth.CurrentUser{OrganizationID: attempt.OrganizationID, Role: authz.MerchantAdmin}
	s.audit(ctx, actor, "service_request", attempt.RequestID, "provider_timed_out", "{}")
	return s.notifyNextProvider(ctx, actor, attempt.RequestID)
}

func (s *Service) AcceptDispatch(ctx context.Context, actor auth.CurrentUser, attemptID uuid.UUID) (Assignment, error) {
	provider, err := s.providerForUser(ctx, actor)
	if err != nil {
		return Assignment{}, err
	}
	var assignment Assignment
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt DispatchAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND provider_id = ?", actor.OrganizationID, attemptID, provider.ID).First(&attempt).Error; err != nil {
			return httperror.NotFound("Request not found")
		}
		if attempt.Status != DispatchNotified {
			return httperror.BadRequest("This request is no longer available")
		}
		if attempt.ExpiresAt.Before(s.now()) {
			return httperror.BadRequest("This request expired")
		}
		var request Request
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", attempt.RequestID).First(&request).Error; err != nil {
			return err
		}
		if request.AssignedProviderID != nil {
			return httperror.Conflict("Another professional already accepted this job")
		}
		now := s.now()
		if err := tx.Model(&DispatchAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{"status": DispatchAccepted, "responded_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&DispatchAttempt{}).Where("request_id = ? AND id <> ? AND status = ?", request.ID, attempt.ID, DispatchNotified).Updates(map[string]any{"status": DispatchCancelled, "updated_at": now}).Error; err != nil {
			return err
		}
		assignment = Assignment{ID: uuid.New(), OrganizationID: actor.OrganizationID, RequestID: request.ID, ProviderID: provider.ID, Status: RequestAssigned, Metadata: "{}"}
		if err := tx.Create(&assignment).Error; err != nil {
			return err
		}
		if err := tx.Model(&Request{}).Where("id = ?", request.ID).Updates(map[string]any{"status": RequestAssigned, "assigned_provider_id": provider.ID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Provider{}).Where("id = ?", provider.ID).Updates(map[string]any{"availability": AvailabilityBusy, "current_job_status": "assigned", "updated_at": now}).Error; err != nil {
			return err
		}
		var handoffID *uuid.UUID
		if request.ConversationSessionID != nil && *request.ConversationSessionID != uuid.Nil && provider.UserID != nil && s.dispatcher != nil {
			id, err := s.dispatcher.AssignHandoff(ctx, actor.OrganizationID, *request.ConversationSessionID, request.CustomerID, *provider.UserID, request.ID)
			if err != nil {
				return err
			}
			handoffID = &id
			if err := tx.Model(&Request{}).Where("id = ?", request.ID).Update("handoff_id", id).Error; err != nil {
				return err
			}
		}
		_ = handoffID
		return tx.Where("id = ?", assignment.ID).Preload("Provider").Preload("Request").First(&assignment).Error
	})
	if err != nil {
		return Assignment{}, err
	}
	s.audit(ctx, actor, "service_request", assignment.RequestID, "provider_accepted", fmt.Sprintf(`{"provider":%q}`, provider.Name))
	s.audit(ctx, actor, "service_request", assignment.RequestID, "assignment_created", "{}")
	s.audit(ctx, actor, "service_request", assignment.RequestID, "conversation_handed_off", "{}")
	request, _ := s.getRequest(ctx, actor.OrganizationID, assignment.RequestID)
	_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, fmt.Sprintf("Great! We've matched you with a professional.\n\nYour technician is %s.\n\nYou can now discuss the job and any additional details here. %s will review your request and provide the final quote.", provider.Name, provider.Name))
	s.saveMessage(ctx, actor.OrganizationID, request.ID, "system", nil, fmt.Sprintf("%s accepted the job.", provider.Name))
	return assignment, nil
}

func (s *Service) DeclineDispatch(ctx context.Context, actor auth.CurrentUser, attemptID uuid.UUID) error {
	provider, err := s.providerForUser(ctx, actor)
	if err != nil {
		return err
	}
	var attempt DispatchAttempt
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ? AND provider_id = ?", actor.OrganizationID, attemptID, provider.ID).First(&attempt).Error; err != nil {
		return httperror.NotFound("Request not found")
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&DispatchAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{"status": DispatchDeclined, "responded_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	s.audit(ctx, actor, "service_request", attempt.RequestID, "provider_declined", "{}")
	return s.notifyNextProvider(ctx, auth.CurrentUser{OrganizationID: actor.OrganizationID, Role: authz.MerchantAdmin}, attempt.RequestID)
}

// TransitionJob moves an accepted job along the provider-driven part of the
// lifecycle. Transitions are validated against jobTransitions so a provider
// cannot skip work states (for example jumping straight to completed) and a
// finished job cannot be completed twice.
func (s *Service) TransitionJob(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID, status, notes string) (Assignment, error) {
	provider, err := s.assignedActor(ctx, actor, requestID)
	if err != nil {
		return Assignment{}, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if !isJobStatus(status) {
		return Assignment{}, httperror.BadRequest("Unsupported job status")
	}
	if status == RequestCancelled && !actor.Role.CanManageOrganization() {
		return Assignment{}, httperror.Forbidden("Only the business can cancel a job")
	}
	var assignment Assignment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ?", actor.OrganizationID, requestID).First(&assignment).Error; err != nil {
		return Assignment{}, httperror.NotFound("Assignment not found")
	}
	request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
	if err != nil {
		return Assignment{}, err
	}
	if request.Status == status {
		return s.loadAssignment(ctx, assignment.ID)
	}
	if !canTransition(request.Status, status) {
		return Assignment{}, httperror.BadRequest(transitionMessage(request.Status, status))
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&Assignment{}).Where("id = ?", assignment.ID).Updates(map[string]any{"status": status, "notes": notes, "updated_at": now}).Error; err != nil {
		return Assignment{}, err
	}
	if err := s.db.WithContext(ctx).Model(&Request{}).Where("id = ? AND status = ?", requestID, request.Status).Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
		return Assignment{}, err
	}
	s.audit(ctx, actor, "service_request", requestID, "job_status_changed", fmt.Sprintf(`{"from":%q,"to":%q}`, request.Status, status))
	if status == RequestCompleted {
		completionMessage := fmt.Sprintf("Your service has been marked as completed.\n\nThank you for using %s.\n\nHow would you rate your experience? Reply with 1 to 5 stars.", s.companyName(ctx, actor.OrganizationID))
		s.saveMessage(ctx, actor.OrganizationID, request.ID, "system", nil, completionMessage)
		_ = s.notify(ctx, actor.OrganizationID, request.CustomerPhone, completionMessage)
		_ = s.db.WithContext(ctx).Model(&Provider{}).Where("id = ?", provider.ID).Updates(map[string]any{"availability": AvailabilityAvailable, "current_job_status": "idle", "jobs_completed": gorm.Expr("jobs_completed + 1"), "updated_at": now}).Error
		s.audit(ctx, actor, "service_request", requestID, "job_completed", "{}")
	}
	if status == RequestCancelled {
		_ = s.db.WithContext(ctx).Model(&Provider{}).Where("id = ?", provider.ID).Updates(map[string]any{"availability": AvailabilityAvailable, "current_job_status": "idle", "updated_at": now}).Error
	}
	return s.loadAssignment(ctx, assignment.ID)
}

func (s *Service) loadAssignment(ctx context.Context, id uuid.UUID) (Assignment, error) {
	var assignment Assignment
	err := s.db.WithContext(ctx).Where("id = ?", id).Preload("Provider").Preload("Request").First(&assignment).Error
	return assignment, err
}

func (s *Service) assignedActor(ctx context.Context, actor auth.CurrentUser, requestID uuid.UUID) (Provider, error) {
	if actor.Role.CanManageOrganization() {
		request, err := s.getRequest(ctx, actor.OrganizationID, requestID)
		if err != nil {
			return Provider{}, err
		}
		if request.AssignedProviderID == nil {
			return Provider{}, httperror.BadRequest("No provider is assigned")
		}
		return s.getProvider(ctx, actor.OrganizationID, *request.AssignedProviderID)
	}
	provider, err := s.providerForUser(ctx, actor)
	if err != nil {
		return Provider{}, err
	}
	var assignment Assignment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND request_id = ? AND provider_id = ?", actor.OrganizationID, requestID, provider.ID).First(&assignment).Error; err != nil {
		return Provider{}, httperror.Forbidden("You are not assigned to this job")
	}
	return provider, nil
}

// jobTransitions is the controlled state machine for a service request once a
// provider has been assigned. Statuses reached by other parts of the system
// (quote_sent by CreateQuote, quote_approved by ApproveQuote, payment_confirmed
// by the payment webhook) are listed as sources so work can continue from them,
// but they are never destinations of a provider-driven transition.
var jobTransitions = map[string][]string{
	RequestAssigned:         {RequestOnTheWay, RequestArrived, RequestCancelled},
	RequestOnTheWay:         {RequestArrived, RequestCancelled},
	RequestArrived:          {RequestInProgress, RequestCancelled},
	RequestInProgress:       {RequestCompleted, RequestCancelled},
	RequestQuoteSent:        {RequestInProgress, RequestCancelled},
	RequestQuoteApproved:    {RequestInProgress, RequestCancelled},
	RequestPaymentConfirmed: {RequestInProgress, RequestCompleted, RequestCancelled},
	RequestCompleted:        {},
	RequestCancelled:        {},
}

func isJobStatus(status string) bool {
	switch status {
	case RequestAssigned, RequestOnTheWay, RequestArrived, RequestInProgress, RequestQuoteSent, RequestQuoteApproved, RequestPaymentConfirmed, RequestCompleted, RequestCancelled:
		return true
	default:
		return false
	}
}

func canTransition(from, to string) bool {
	for _, allowed := range jobTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func transitionMessage(from, to string) string {
	if from == RequestCompleted || from == RequestCancelled {
		return "This job is already " + strings.ReplaceAll(from, "_", " ") + "."
	}
	allowed := jobTransitions[from]
	if len(allowed) == 0 {
		return "This job cannot be updated right now."
	}
	readable := make([]string, 0, len(allowed))
	for _, value := range allowed {
		readable = append(readable, strings.ReplaceAll(value, "_", " "))
	}
	return fmt.Sprintf("A job that is %s can only move to: %s.", strings.ReplaceAll(from, "_", " "), strings.Join(readable, ", "))
}

func (s *Service) companyName(ctx context.Context, organizationID uuid.UUID) string {
	settings, err := s.ensureSettings(ctx, organizationID)
	if err != nil || settings.CompanyDisplayName == "" {
		return "our team"
	}
	return settings.CompanyDisplayName
}

func displayLocation(request Request) string {
	if request.Area != "" && request.Address != "" {
		return request.Area + " · " + request.Address
	}
	if request.Area != "" {
		return request.Area
	}
	return request.Address
}

func clip(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mustUUID(raw string) uuid.UUID {
	id, _ := uuid.Parse(raw)
	return id
}
