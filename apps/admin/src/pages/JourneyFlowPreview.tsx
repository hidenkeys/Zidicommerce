type Component = { type: string; name?: string; label?: string; "input-type"?: string; "data-source"?: string | { id: string; title: string }[]; children?: Component[] };
type Screen = { id: string; title: string; layout: { children: Component[] } };

// Render the compiler's supported input component rather than a second form
// definition. All navigation, validation and actions remain on the backend.
export function JourneyFlowPreview({ compilation, screenId, prompt, options, busy, submit }: {
  compilation?: Record<string, unknown>; screenId?: string; prompt?: string;
  options?: { id: string; label: string }[]; busy: boolean; submit: (id: string, label: string) => void;
}) {
  const screen = (compilation?.screens as Screen[] | undefined)?.find(s => s.id === screenId);
  const form = screen?.layout.children.find(c => c.type === "Form");
  const input = form?.children?.find(c => ["Dropdown", "RadioButtonsGroup", "TextInput", "DatePicker"].includes(c.type));
  const choices = input?.type === "RadioButtonsGroup" && Array.isArray(input["data-source"]) ? input["data-source"].map(o => ({ id: o.id, label: o.title })) : options;
  return <div className="journey-flow-screen" data-testid="compiled-flow-screen" data-screen={screen?.id}>
    <small>{screen?.title ?? "Request received"}</small><h3>{prompt}</h3>
    {choices?.map(option => <button key={option.id} type="button" className="journey-flow-choice" disabled={busy} onClick={() => submit(option.id, option.label)}>{option.label}</button>)}
  </div>;
}
