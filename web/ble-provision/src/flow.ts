export type Step = "connect" | "auth" | "wifi" | "result";
export type FlowEvent = "connected" | "authenticated" | "submitting" | "retry" | "unauthorized" | "disconnected";

export function nextStep(current: Step, event: FlowEvent): Step {
  if (event === "disconnected") return "connect";
  if (event === "connected") return "auth";
  if (event === "unauthorized") return "auth";
  if (event === "authenticated" && current === "auth") return "wifi";
  if (event === "submitting" && current === "wifi") return "result";
  if (event === "retry" && current === "result") return "auth";
  return current;
}
