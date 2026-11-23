export const terminalFontSize = (width: number) => width < 768 ? 12 : 13
export interface TerminalOwnership { owner: boolean; cols: number; rows: number }
export interface TerminalControl { handoff: () => void; release: () => void }
