# ZidiCommerce Git Workflow

This repository should build a readable history: one feature branch per unit of work, one verified commit or small commit set per finished feature, then push.

## Branches

Use short, descriptive branch names:

```bash
git switch -c feature/whatsapp-template-health
```

Use `codex/` branches for Codex-led implementation work and `feature/` branches for normal product work. Keep long-running branches rare.

## Feature Loop

1. Start from the latest shared branch.
2. Create a branch for one feature or fix.
3. Make the smallest coherent implementation.
4. Run the local check before committing:

```bash
npm run check
```

5. Review your diff:

```bash
git status --short
git diff
```

6. Commit with a clear message:

```bash
git add <files>
git commit -m "Add WhatsApp template health metrics"
```

7. Push the branch:

```bash
git push -u origin <branch>
```

## Definition Of Done

A feature is done when:

- API tests pass.
- Admin and Field builds pass.
- The diff contains only intentional files.
- Migrations, environment variables, and docs are updated when behavior changes.
- Any generated artifacts are excluded unless they are reviewed fixtures.
- The branch is pushed after the commit.

## Commit Style

Prefer messages that say what changed:

```text
Add channel health metrics to admin
Harden WhatsApp webhook signature errors
Split order fulfilment transition tests
```

Avoid vague messages like:

```text
updates
fixes
more changes
```

## Railway Discipline

Deploy only after the branch is committed and pushed. Use staging before production, and record the exact commit SHA when a deployment matters for debugging.

Do not commit local Railway link metadata or secret values.

