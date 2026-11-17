import {vi} from "vitest";

/** The diagnostic-report calls as spies. apiMock.ts spreads these into its `api` and resets them. */
export const diagApi = {
    DiagPreview: vi.fn(), DiagSave: vi.fn(),
};

/** What the Go side would return: a small, already scrubbed report. */
export const sampleReport = `{
  "schema": 1,
  "kind": "winforge-diagnostic-report",
  "winforge": {"version": "v0.0.0-test", "build": "go1.26 windows/amd64"}
}
`;

export function resetDiag(): void {
    for (const fn of Object.values(diagApi)) fn.mockReset();
    diagApi.DiagPreview.mockResolvedValue(sampleReport);
    diagApi.DiagSave.mockResolvedValue("C:\\Reports\\winforge-diagnostics.json");
}
