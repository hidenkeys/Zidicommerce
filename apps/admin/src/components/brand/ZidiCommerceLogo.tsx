type ZidiCommerceLogoProps = {
  className?: string;
  compact?: boolean;
  tone?: "dark" | "light";
  subtitle?: string;
};

const markPath = "M12 12H52L44 22H12ZM40 26H52L24 38H12ZM20 42H52V52H12Z";
const wordPath = "M0 0H29V6L9 30H29V37H0V31L20 7H0Z M35 12H42V37H35Z M35 1H42V8H35Z M67 0H74V37H67V34C64 37 61 38 57 38C48 38 44 32 44 25C44 17 49 11 57 11C61 11 65 13 67 15ZM67 25C67 20 64 17 60 17C55 17 52 20 52 25C52 29 55 32 60 32C64 32 67 29 67 25Z M81 12H88V37H81Z M81 1H88V8H81Z";
const descriptor = [{"x": 0, "d": "M10 1L8 3C4 0 2 3 2 7C2 11 5 13 8 10L10 12C5 17 0 13 0 7C0 1 5 -2 10 1Z"}, {"x": 10.25, "d": "M4.5 3C10 3 10 14 4.5 14C-1 14 -1 3 4.5 3ZM4.5 5.5C2 5.5 2 11.5 4.5 11.5C7 11.5 7 5.5 4.5 5.5Z"}, {"x": 19.5, "d": "M0 4H2.5V5C4 3 6 3 7.5 5C10 2 14 3 14 7V14H11.5V7.5C11.5 5 8.5 5 8.5 7.5V14H6V7.5C6 5 2.5 5 2.5 7.5V14H0Z"}, {"x": 33.75, "d": "M0 4H2.5V5C4 3 6 3 7.5 5C10 2 14 3 14 7V14H11.5V7.5C11.5 5 8.5 5 8.5 7.5V14H6V7.5C6 5 2.5 5 2.5 7.5V14H0Z"}, {"x": 48.0, "d": "M9 9.5H2.5C3 12 6 12.5 8 10.5L9 12.5C5 16 0 14 0 8.5C0 1.5 10 1 9 9.5ZM2.5 7.5H6.5C6 4.5 3 4.5 2.5 7.5Z"}, {"x": 57.25, "d": "M0 4H2.5V5.5C3.5 3.5 5 3.5 6 4V6.5C4 6 2.5 7 2.5 9V14H0Z"}, {"x": 63.5, "d": "M8 5L6.5 7C3 4 1 8 3 11C4 12.5 6 12 7 11L8.5 13C4 16 0 13.5 0 9C0 4 4 1.5 8 5Z"}, {"x": 71.75, "d": "M9 9.5H2.5C3 12 6 12.5 8 10.5L9 12.5C5 16 0 14 0 8.5C0 1.5 10 1 9 9.5ZM2.5 7.5H6.5C6 4.5 3 4.5 2.5 7.5Z"}];

export function ZidiCommerceSymbol({ className = "", tone = "dark" }: Pick<ZidiCommerceLogoProps, "className" | "tone">) {
  return <svg className={className} viewBox="0 0 64 64" role="img" aria-label="Zidi Commerce">
    <path d={markPath} fill={tone === "light" ? "var(--z-brand)" : "var(--z-on-brand)"} />
  </svg>;
}

export function ZidiCommerceLogo({ className = "", compact = false, tone = "dark", subtitle }: ZidiCommerceLogoProps) {
  if (compact) return <ZidiCommerceSymbol className={className} tone={tone} />;
  return <div className={`zidi-commerce-logo ${className}`.trim()}>
    <span aria-hidden="true" style={{ display: "flex", flex: "0 0 auto", margin: 0 }}><ZidiCommerceSymbol className="zidi-commerce-logo__symbol" tone={tone} /></span>
    <div className="zidi-commerce-logo__wordmark">
      <svg viewBox="0 0 100 64" className="zidi-commerce-logo__lettering" width="100" height="64" role="img" aria-label="Zidi Commerce" fill={tone === "light" ? "var(--z-neutral-50)" : "var(--z-on-brand)"} fillRule="evenodd">
        <path d={wordPath} />
        <g transform="translate(0 41) scale(1.08)">{descriptor.map(({ x, d }, i) => <path key={i} transform={`translate(${x} 0)`} d={d} />)}</g>
      </svg>
      {subtitle ? <small>{subtitle}</small> : null}
    </div>
  </div>;
}
