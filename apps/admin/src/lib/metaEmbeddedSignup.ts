export type MetaSignupLaunch = {
  app_id: string;
  configuration_id: string;
  graph_api_version: string;
};

export type MetaSignupResult = {
  authorizationCode: string;
  whatsappBusinessAccountID: string;
  phoneNumberID: string;
};

type FacebookLoginResponse = {
  authResponse?: { code?: string };
  status?: string;
};

type FacebookSDK = {
  init(options: { appId: string; autoLogAppEvents: boolean; xfbml: boolean; version: string }): void;
  login(callback: (response: FacebookLoginResponse) => void, options: Record<string, unknown>): void;
};

declare global {
  interface Window {
    FB?: FacebookSDK;
    fbAsyncInit?: () => void;
  }
}

let sdkPromise: Promise<FacebookSDK> | null = null;

function loadFacebookSDK(launch: MetaSignupLaunch): Promise<FacebookSDK> {
  if (window.FB) {
    window.FB.init({ appId: launch.app_id, autoLogAppEvents: true, xfbml: true, version: launch.graph_api_version });
    return Promise.resolve(window.FB);
  }
  if (sdkPromise) return sdkPromise;
  sdkPromise = new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      sdkPromise = null;
      reject(new Error("Meta sign-in could not be loaded. Check your connection and try again."));
    }, 15000);
    const previous = window.fbAsyncInit;
    window.fbAsyncInit = () => {
      previous?.();
      if (!window.FB) {
        window.clearTimeout(timeout);
        sdkPromise = null;
        reject(new Error("Meta sign-in is unavailable."));
        return;
      }
      window.FB.init({ appId: launch.app_id, autoLogAppEvents: true, xfbml: true, version: launch.graph_api_version });
      window.clearTimeout(timeout);
      resolve(window.FB);
    };
    if (!document.getElementById("facebook-jssdk")) {
      const script = document.createElement("script");
      script.id = "facebook-jssdk";
      script.async = true;
      script.defer = true;
      script.crossOrigin = "anonymous";
      script.src = "https://connect.facebook.net/en_US/sdk.js";
      script.onerror = () => {
        window.clearTimeout(timeout);
        sdkPromise = null;
        reject(new Error("Meta sign-in could not be loaded. Check your connection and try again."));
      };
      document.body.appendChild(script);
    }
  });
  return sdkPromise;
}

function embeddedMessageData(event: MessageEvent): Record<string, unknown> | null {
  if (event.origin !== "https://www.facebook.com" && event.origin !== "https://web.facebook.com") return null;
  try {
    const value = typeof event.data === "string" ? JSON.parse(event.data) : event.data;
    return value && typeof value === "object" ? value as Record<string, unknown> : null;
  } catch {
    return null;
  }
}

export async function launchMetaEmbeddedSignup(launch: MetaSignupLaunch): Promise<MetaSignupResult> {
  const sdk = await loadFacebookSDK(launch);
  return new Promise((resolve, reject) => {
    let authorizationCode = "";
    let whatsappBusinessAccountID = "";
    let phoneNumberID = "";
    let settled = false;

    const cleanup = () => {
      window.clearTimeout(timeout);
      window.removeEventListener("message", receiveMessage);
    };
    const fail = (message: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error(message));
    };
    const finish = () => {
      if (settled || !authorizationCode || !whatsappBusinessAccountID || !phoneNumberID) return;
      settled = true;
      cleanup();
      resolve({ authorizationCode, whatsappBusinessAccountID, phoneNumberID });
    };
    const receiveMessage = (event: MessageEvent) => {
      const payload = embeddedMessageData(event);
      if (!payload || payload.type !== "WA_EMBEDDED_SIGNUP") return;
      const eventName = String(payload.event ?? "");
      if (eventName === "CANCEL") {
        fail("Meta signup was cancelled before a WhatsApp account was connected.");
        return;
      }
      if (eventName === "ERROR") {
        fail("Meta could not complete WhatsApp signup. Review the Meta dialog and try again.");
        return;
      }
      if (eventName !== "FINISH") return;
      const data = payload.data && typeof payload.data === "object" ? payload.data as Record<string, unknown> : {};
      whatsappBusinessAccountID = String(data.waba_id ?? "").trim();
      phoneNumberID = String(data.phone_number_id ?? "").trim();
      if (!whatsappBusinessAccountID || !phoneNumberID) {
        fail("Meta signup finished without a WhatsApp account and phone number.");
        return;
      }
      finish();
    };
    const timeout = window.setTimeout(() => fail("Meta signup did not finish in time. Close the Meta window and try again."), 5 * 60 * 1000);
    window.addEventListener("message", receiveMessage);

    sdk.login((response) => {
      authorizationCode = String(response.authResponse?.code ?? "").trim();
      if (!authorizationCode) {
        fail(response.status === "not_authorized" ? "Meta authorization was not granted." : "Meta signup was closed before authorization completed.");
        return;
      }
      finish();
    }, {
      config_id: launch.configuration_id,
      response_type: "code",
      override_default_response_type: true,
      extras: { setup: {}, featureType: "", sessionInfoVersion: "3" },
    });
  });
}
