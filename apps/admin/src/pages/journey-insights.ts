type InspectableStep = {
  key: string;
  title: string;
  kind: string;
  next?: string;
  failure?: string;
  destination?: string;
  required?: boolean;
  text?: string;
  options?: { next?: string }[];
};

export function inspectJourney(start: string, steps: InspectableStep[]) {
  const indexed = new Map(steps.map((step) => [step.key, step]));
  const seen = new Set<string>();
  const pending = [start];
  while (pending.length) {
    const key = pending.pop()!;
    const step = indexed.get(key);
    if (!step || seen.has(key)) continue;
    seen.add(key);
    if (["end", "handoff"].includes(step.kind)) continue;
    for (const next of [
      step.next,
      step.failure,
      ...(step.options ?? []).map((o) => o.next),
    ]) {
      if (next && !seen.has(next)) pending.push(next);
    }
  }
  return steps.flatMap((step) => {
    const issues: { key: string; message: string }[] = [];
    if (!seen.has(step.key))
      issues.push({
        key: step.key,
        message: "No route from the starting step reaches " + step.title + ".",
      });
    if (
      ["buttons", "list", "question"].includes(step.kind) &&
      !step.text?.trim()
    ) {
      issues.push({
        key: step.key,
        message: "Add the question customers will see at " + step.title + ".",
      });
    }
    if (step.kind === "condition" && (!step.next || !step.failure)) {
      issues.push({
        key: step.key,
        message:
          "Set both the matched and unmatched routes for " + step.title + ".",
      });
    }
    return issues;
  });
}
