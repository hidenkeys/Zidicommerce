import { useId, useState, type InputHTMLAttributes } from "react";

export function PasswordField({label, ...props}: InputHTMLAttributes<HTMLInputElement> & {label: string}) {
  const [visible,setVisible] = useState(false);
  const id = useId();
  return <div className="password-field">
    <label htmlFor={id}>{label}</label>
    <div><input {...props} id={id} type={visible ? "text" : "password"}/>
      <button type="button" aria-controls={id} aria-label={visible ? "Hide password" : "Show password"} onClick={() => setVisible(!visible)}>{visible ? "Hide" : "Show"}</button>
    </div>
  </div>;
}
