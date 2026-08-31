type ZidiCommerceLogoProps = {
  className?: string;
  compact?: boolean;
  tone?: "dark" | "light";
  subtitle?: string;
};

export function ZidiCommerceSymbol({ className = "", tone = "dark" }: Pick<ZidiCommerceLogoProps, "className" | "tone">) {
  const ink = tone === "light" ? "#ffffff" : "#231f20";
  const accent = tone === "light" ? "#ffcc22" : "#231f20";
  const panel = tone === "light" ? "#1d2633" : "#fff7d2";

  return (
    <svg className={className} viewBox="0 0 64 64" role="img" aria-label="Zidi Commerce">
      <rect width="64" height="64" rx="13" fill="#ffcc22" />
      <path d="M17 21h30l5 9H12l5-9Z" fill={ink} />
      <path d="M15 30h34v21H15V30Z" fill={ink} />
      <rect x="20" y="35" width="9" height="9" rx="2" fill={panel} />
      <rect x="35" y="35" width="9" height="9" rx="2" fill={panel} />
      <path d="M22 18h22" fill="none" stroke={ink} strokeWidth="4.5" strokeLinecap="round" />
      <path d="M22 18h22" fill="none" stroke={accent} strokeWidth="2" strokeLinecap="round" />
      <path d="M21 48h19" fill="none" stroke={panel} strokeWidth="3" strokeLinecap="round" />
      <circle cx="48" cy="48" r="6.3" fill={accent} />
      <path d="m45.2 48 1.8 1.8 4-4.3" fill="none" stroke={panel} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function ZidiCommerceLogo({ className = "", compact = false, tone = "dark", subtitle }: ZidiCommerceLogoProps) {
  if (compact) {
    return <ZidiCommerceSymbol className={className} tone={tone} />;
  }

  return (
    <div className={`zidi-commerce-logo ${className}`.trim()}>
      <ZidiCommerceSymbol className="zidi-commerce-logo__symbol" tone={tone} />
      <div className="zidi-commerce-logo__wordmark" aria-label="Zidi Commerce">
        <strong>
          <span>Zidi</span>
          <span>Commerce</span>
        </strong>
        {subtitle ? <small>{subtitle}</small> : null}
      </div>
    </div>
  );
}
