import type { ReactNode } from "react";
import { ArrowLeft, BatteryFull, MoreVertical, Paperclip, Signal, Smile, Wifi, Mic } from "lucide-react";
import "./whatsapp-preview.css";

export function WhatsAppPhone({ businessName, children, label = "Preview only" }: { businessName: string; children: ReactNode; label?: string }) {
  return <section className="wa-phone" aria-label={`${businessName} WhatsApp preview`}>
    <div className="wa-status" aria-hidden="true"><span>9:41</span><span><Signal size={12}/><Wifi size={12}/><BatteryFull size={17}/></span></div>
    <header className="wa-header"><ArrowLeft size={19} aria-hidden="true"/><span className="wa-avatar" aria-hidden="true">{businessName.slice(0, 1).toUpperCase()}</span><div><strong>{businessName}</strong><small>Business account · {label}</small></div><MoreVertical size={19} aria-hidden="true"/></header>
    <div className="wa-wallpaper">{children}</div>
    <div className="wa-composer" aria-label="Disabled preview composer"><div><Smile size={19} aria-hidden="true"/><input disabled placeholder="Preview · messages aren’t sent" aria-label="Preview only, messages are not sent"/><Paperclip size={18} aria-hidden="true"/></div><Mic size={20} aria-hidden="true"/></div>
    <div className="wa-home" aria-hidden="true"><span/></div>
  </section>;
}
