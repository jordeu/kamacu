import { createContext, useContext } from "react";

export const CatchUpOpenContext = createContext(false);
export function useIsCatchUpOpen() {
  return useContext(CatchUpOpenContext);
}

export const CatchUpContext = createContext<(() => void) | null>(null);
export function useOpenCatchUp() {
  const open = useContext(CatchUpContext);
  if (!open) throw new Error("CatchUpProvider is required");
  return open;
}
