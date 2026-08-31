package bot

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(router fiber.Router) {
	router.Get("/bot-modules", h.systemModules)
	router.Get("/bot-actions", h.systemActions)
	router.Get("/bot-question-types", h.questionTypes)
	router.Get("/bot-setup/status", h.setupStatus)
	router.Get("/bot-workflow", h.getCommerceWorkflowConfiguration)
	router.Put("/bot-workflow", h.updateCommerceWorkflowConfiguration)
	router.Get("/knowledge-entries", h.listKnowledgeEntries)
	router.Post("/knowledge-entries", h.createKnowledgeEntry)
	router.Get("/knowledge-entries/:id", h.getKnowledgeEntry)
	router.Patch("/knowledge-entries/:id", h.updateKnowledgeEntry)
	router.Delete("/knowledge-entries/:id", h.archiveKnowledgeEntry)
	router.Get("/document-sources", h.listDocumentSources)
	router.Post("/document-sources", h.createDocumentSource)
	router.Get("/document-sources/:id", h.getDocumentSource)
	router.Patch("/document-sources/:id", h.updateDocumentSource)
	router.Delete("/document-sources/:id", h.archiveDocumentSource)
	router.Get("/document-sources/:id/chunks", h.listDocumentChunks)
	router.Patch("/document-chunks/:id", h.updateDocumentChunk)
	router.Post("/document-chunks/:id/approve", h.approveDocumentChunk)
	router.Delete("/document-chunks/:id", h.archiveDocumentChunk)
	router.Get("/bot-faqs", h.listFAQs)
	router.Post("/bot-faqs", h.createFAQ)
	router.Patch("/bot-faqs/:id", h.updateFAQ)
	router.Post("/bot-faqs/match", h.matchFAQ)

	router.Get("/bots", h.listBots)
	router.Post("/bots", h.createBot)
	router.Post("/bots/self-service", h.createSelfServiceBot)
	router.Post("/bots/service-booking", h.createServiceBookingBot)
	router.Get("/bots/:id", h.getBot)
	router.Patch("/bots/:id", h.updateBot)
	router.Get("/bots/:id/share-link", h.shareLink)
	router.Get("/bots/:id/versions", h.listVersions)
	router.Post("/bots/:id/versions", h.createVersion)

	router.Get("/bot-versions/:id/configuration", h.getConfiguration)
	router.Get("/bot-versions/:id/modules", h.listModules)
	router.Post("/bot-versions/:id/modules", h.addModule)
	router.Post("/bot-versions/:id/modules/reorder", h.reorderModules)
	router.Get("/bot-versions/:id/variables", h.listVariables)
	router.Post("/bot-versions/:id/variables", h.createVariable)
	router.Get("/bot-versions/:id/questions", h.listQuestions)
	router.Post("/bot-versions/:id/questions", h.createQuestion)
	router.Get("/bot-versions/:id/actions", h.listActions)
	router.Post("/bot-versions/:id/actions", h.createAction)
	router.Get("/bot-versions/:id/conditions", h.listConditions)
	router.Post("/bot-versions/:id/conditions", h.createCondition)
	router.Get("/bot-versions/:id/integrations", h.listIntegrations)
	router.Post("/bot-versions/:id/integrations", h.createIntegration)
	router.Get("/bot-versions/:id/steps", h.listSteps)
	router.Post("/bot-versions/:id/steps", h.createStep)
	router.Post("/bot-versions/:id/validate", h.validateVersion)
	router.Post("/bot-versions/:id/publish", h.publishVersion)
	router.Get("/bot-versions/:id/preview", h.previewVersion)

	router.Patch("/bot-steps/:id", h.updateStep)
	router.Patch("/bot-modules/:id", h.updateModule)
	router.Post("/bot-modules/:id/enable", h.enableModule)
	router.Post("/bot-modules/:id/disable", h.disableModule)
}

func (h *Handler) systemModules(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": SystemModules()})
}

func (h *Handler) systemActions(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": SystemActions()})
}

func (h *Handler) questionTypes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": QuestionTypes()})
}

func (h *Handler) listBots(c *fiber.Ctx) error {
	data, err := h.service.ListBots(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) createBot(c *fiber.Ctx) error {
	var input BotInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateBot(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) createSelfServiceBot(c *fiber.Ctx) error {
	var input SelfServiceBotInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateSelfServiceBot(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) createServiceBookingBot(c *fiber.Ctx) error {
	var input ServiceBookingBotInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateServiceBookingBot(c.UserContext(), currentUser(c), input.Name, input.WelcomeMessage)
	return respond(c, data, err)
}

func (h *Handler) getBot(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetBot(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateBot(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input BotInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateBot(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) shareLink(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetShareLink(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) setupStatus(c *fiber.Ctx) error {
	data, err := h.service.GetSetupStatus(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) getCommerceWorkflowConfiguration(c *fiber.Ctx) error {
	data, err := h.service.GetCommerceWorkflowConfiguration(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) updateCommerceWorkflowConfiguration(c *fiber.Ctx) error {
	var input CommerceWorkflowConfigurationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateCommerceWorkflowConfiguration(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listVersions(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListVersions(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createVersion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input VersionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateVersion(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) getConfiguration(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetConfiguration(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) addModule(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ModuleInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.AddModule(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listModules(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListModules(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateModule(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ModuleInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateModule(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) enableModule(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.SetModuleEnabled(c.UserContext(), currentUser(c), id, true)
	return respond(c, data, err)
}

func (h *Handler) disableModule(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.SetModuleEnabled(c.UserContext(), currentUser(c), id, false)
	return respond(c, data, err)
}

func (h *Handler) reorderModules(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ModuleReorderInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.ReorderModules(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createVariable(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input VariableInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateVariable(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listVariables(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListVariables(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createQuestion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input QuestionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateQuestion(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listQuestions(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListQuestions(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createAction(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ActionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateAction(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listActions(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListActions(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createCondition(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ConditionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateCondition(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listConditions(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListConditions(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createIntegration(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input IntegrationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateIntegration(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listIntegrations(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListIntegrations(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createStep(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input StepInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateStep(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) updateStep(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input StepInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateStep(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listSteps(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListSteps(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) validateVersion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ValidateVersion(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) publishVersion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.PublishVersion(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) previewVersion(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.PreviewVersion(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listFAQs(c *fiber.Ctx) error {
	data, err := h.service.ListFAQs(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) createFAQ(c *fiber.Ctx) error {
	var input FAQInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateFAQ(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateFAQ(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input FAQInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateFAQ(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listKnowledgeEntries(c *fiber.Ctx) error {
	filter := KnowledgeEntryFilter{
		Kind:     c.Query("kind"),
		Category: c.Query("category"),
		Status:   c.Query("status"),
		Search:   c.Query("search"),
	}
	data, err := h.service.ListKnowledgeEntries(c.UserContext(), currentUser(c), filter)
	return respond(c, data, err)
}

func (h *Handler) getKnowledgeEntry(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetKnowledgeEntry(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createKnowledgeEntry(c *fiber.Ctx) error {
	var input KnowledgeEntryInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateKnowledgeEntry(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateKnowledgeEntry(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input KnowledgeEntryInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateKnowledgeEntry(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) archiveKnowledgeEntry(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ArchiveKnowledgeEntry(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listDocumentSources(c *fiber.Ctx) error {
	filter := DocumentSourceFilter{
		SourceType: c.Query("source_type"),
		Status:     c.Query("status"),
		Search:     c.Query("search"),
	}
	data, err := h.service.ListDocumentSources(c.UserContext(), currentUser(c), filter)
	return respond(c, data, err)
}

func (h *Handler) getDocumentSource(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetDocumentSource(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createDocumentSource(c *fiber.Ctx) error {
	var input DocumentSourceInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateDocumentSource(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateDocumentSource(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input DocumentSourceInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateDocumentSource(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) archiveDocumentSource(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	options := DocumentSourceArchiveOptions{ArchiveLinkedKnowledge: boolQuery(c.Query("archive_linked_knowledge"))}
	data, err := h.service.ArchiveDocumentSource(c.UserContext(), currentUser(c), id, options)
	return respond(c, data, err)
}

func (h *Handler) listDocumentChunks(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListDocumentChunks(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateDocumentChunk(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input DocumentChunkInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateDocumentChunk(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) approveDocumentChunk(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input DocumentChunkApprovalInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.ApproveDocumentChunk(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) archiveDocumentChunk(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ArchiveDocumentChunk(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) matchFAQ(c *fiber.Ctx) error {
	var input struct {
		Query string `json:"query"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.MatchFAQ(c.UserContext(), currentUser(c), input.Query)
	return respond(c, data, err)
}

func currentUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func bind(c *fiber.Ctx, target any) error {
	if err := c.BodyParser(target); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	return nil
}

func boolQuery(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "true" || value == "1" || value == "yes"
}

func paramID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid ID")
	}
	return id, nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
