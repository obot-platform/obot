import { createContext, useContext, useSyncExternalStore } from "react";

export const DEFAULT_OBOT_URL = "https://obot.example.com";

const STORAGE_KEY = "obot-docs-url";
const CHANGE_EVENT = "obot-docs-url-change";

export const ObotUrlExamplesContext = createContext(false);

export function normalizeObotUrl(value: string): string {
  const normalized = value.trim().replace(/\/+$/, "");
  const url = new URL(normalized);

  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("Enter a URL beginning with http:// or https://.");
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error("Enter an Obot base URL without credentials, a query, or a fragment.");
  }

  return normalized;
}

function getStoredObotUrl(): string {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return stored ? normalizeObotUrl(stored) : DEFAULT_OBOT_URL;
  } catch {
    return DEFAULT_OBOT_URL;
  }
}

function subscribe(listener: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, listener);
  window.addEventListener("storage", listener);

  return () => {
    window.removeEventListener(CHANGE_EVENT, listener);
    window.removeEventListener("storage", listener);
  };
}

export function useObotUrl(): string {
  return useSyncExternalStore(subscribe, getStoredObotUrl, () => DEFAULT_OBOT_URL);
}

export function useObotUrlExamples(): boolean {
  return useContext(ObotUrlExamplesContext);
}

export function saveObotUrl(value: string): string {
  const normalized = normalizeObotUrl(value);
  window.localStorage.setItem(STORAGE_KEY, normalized);
  window.dispatchEvent(new Event(CHANGE_EVENT));
  return normalized;
}
