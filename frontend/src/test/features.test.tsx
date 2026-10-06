import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const step = (id: string, name: string) => ({kind: "app" as const, id, name});

/** Mocks a test wants must be set before this; only the state is swapped in here. */
async function openApp(state = makeState()) {
    api.GetState.mockImplementation(async () => structuredClone(state));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    return user;
}

async function go(user: ReturnType<typeof userEvent.setup>, tab: RegExp) {
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: tab}));
}

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

/** Runs Firefox and fails it with the given reason, leaving the finished dialog open. */
async function failFirefox(user: ReturnType<typeof userEvent.setup>, reason: string, state = makeState()) {
    await go(user, /Catalog/);
    const card = screen.getByRole("button", {name: "Firefox"}).closest("article") as HTMLElement;
    await user.click(within(card).getByRole("button", {name: "Install"}));
    await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
    await emit.install({step: step("firefox", "Firefox"), status: "start"});
    await emit.install({step: step("firefox", "Firefox"), status: "failed", error: "winget exited with code 0x8a150049", reason});
    await emit.done(["firefox"]);
    void state;
}

describe("when a step fails", () => {
    it("says what to do about it, and retries only what failed", async () => {
        const user = await openApp();
        await failFirefox(user, "network");
        expect(await screen.findByText("The download failed. Check your connection, then retry.")).toBeInTheDocument();
        api.PlanPending.mockResolvedValue({steps: [step("firefox", "Firefox")], alreadyInstalled: [], needsAdmin: false});
        await user.click(screen.getByRole("button", {name: "Retry what failed"}));
        await waitFor(() => expect(api.ResumePending).toHaveBeenCalledTimes(1));
        expect(api.PlanPending).toHaveBeenCalled();
    });

    it("offers a restart as administrator instead when that is the cause", async () => {
        const user = await openApp();
        await failFirefox(user, "admin");
        expect(await screen.findByText(/Needs administrator rights/)).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Retry what failed"})).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Restart as administrator and continue"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("does not offer the admin restart to someone who already is administrator", async () => {
        const user = await openApp(makeState({admin: true}));
        await failFirefox(user, "admin");
        await screen.findByText(/Needs administrator rights/);
        expect(screen.queryByRole("button", {name: "Restart as administrator and continue"})).not.toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Retry what failed"})).toBeInTheDocument();
    });

    it("shows download progress on the step that is running", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const card = screen.getByRole("button", {name: "Firefox"}).closest("article") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Install"}));
        await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        await emit.install({step: step("firefox", "Firefox"), status: "output", line: "  42%", percent: 42});
        expect(await screen.findByRole("progressbar", {name: "42%"})).toBeInTheDocument();
        await emit.install({step: step("firefox", "Firefox"), status: "ok"});
        expect(screen.queryByRole("progressbar", {name: "42%"})).not.toBeInTheDocument();
    });

    it("tells the person to restart Windows when a step needs it", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const card = screen.getByRole("button", {name: "Firefox"}).closest("article") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Install"}));
        await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "ok", reason: "reboot"});
        await emit.done([]);
        expect(await screen.findByText(/Restart your PC, then open WinForge/)).toBeInTheDocument();
    });
});

describe("an unfinished run", () => {
    const pending = {title: "Gaming", steps: [step("firefox", "Firefox"), step("vlc", "VLC")]};

    it("is offered on start, reviewed before it runs, and can be dismissed", async () => {
        const user = await openApp(makeState({pending}));
        const banner = await screen.findByRole("region", {name: "A previous run did not finish"});
        expect(banner).toHaveTextContent("2 steps are left from “Gaming”");
        api.PlanPending.mockResolvedValue({steps: pending.steps, alreadyInstalled: [], needsAdmin: false});
        await user.click(within(banner).getByRole("button", {name: "Continue"}));
        const dialog = await screen.findByRole("dialog");
        expect(within(dialog).getByText("Firefox")).toBeInTheDocument();
        expect(api.ResumePending).not.toHaveBeenCalled();
        await user.click(within(dialog).getByRole("button", {name: "Run 2 steps"}));
        expect(api.ResumePending).toHaveBeenCalledTimes(1);
    });

    it("is forgotten when dismissed", async () => {
        const user = await openApp(makeState({pending}));
        await user.click(await screen.findByRole("button", {name: "Dismiss"}));
        expect(api.DiscardPending).toHaveBeenCalled();
    });

    it("is not offered when there is nothing left to do", async () => {
        await openApp();
        expect(screen.queryByRole("region", {name: "A previous run did not finish"})).not.toBeInTheDocument();
    });
});

describe("uninstalling", () => {
    it("asks first, then plans and runs the removal", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(screen.getByRole("button", {name: "VLC"})); // installed in the fixture
        const panel = await screen.findByRole("dialog");
        await user.click(within(panel).getByRole("button", {name: "Uninstall"}));
        const sure = await screen.findByRole("alertdialog", {name: "Uninstall “VLC”?"});
        expect(api.ApplyUninstall).not.toHaveBeenCalled();
        await user.click(within(sure).getByRole("button", {name: "Uninstall"}));
        await waitFor(() => expect(api.PlanUninstall).toHaveBeenCalledWith(["vlc"]));
        await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
        expect(api.ApplyUninstall).toHaveBeenCalledWith(["vlc"]);
    });

    it("keeps the app if the person says no", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(screen.getByRole("button", {name: "VLC"}));
        await user.click(within(await screen.findByRole("dialog")).getByRole("button", {name: "Uninstall"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Cancel"}));
        expect(api.PlanUninstall).not.toHaveBeenCalled();
    });
});

describe("winget missing", () => {
    it("offers to set it up and rescans", async () => {
        const user = await openApp(makeState({wingetError: "winget not found"}));
        const scans = api.GetState.mock.calls.length;
        await user.click(await screen.findByRole("button", {name: "Set up winget"}));
        expect(api.InstallWinget).toHaveBeenCalled();
        await waitFor(() => expect(api.GetState.mock.calls.length).toBeGreaterThan(scans));
    });

    it("shows why when it cannot", async () => {
        api.InstallWinget.mockRejectedValueOnce(new Error("opened the Store page"));
        const user = await openApp(makeState({wingetError: "winget not found"}));
        await user.click(await screen.findByRole("button", {name: "Set up winget"}));
        expect(await screen.findByText("opened the Store page")).toBeInTheDocument();
    });
});

describe("share codes", () => {
    it("copies a profile as one line of text", async () => {
        api.ShareCode.mockResolvedValue("WF1.abc");
        const user = await openApp();
        await go(user, /Profiles/);
        await user.click(screen.getByRole("button", {name: /Mine/}));
        await user.click(await screen.findByRole("button", {name: /More/}));
        await user.click(screen.getByRole("menuitem", {name: "Copy share code"}));
        await waitFor(async () => expect(await navigator.clipboard.readText()).toBe("WF1.abc"));
        expect(await screen.findByText("Share code copied. Paste it in a message.")).toBeInTheDocument();
    });

    it("imports a pasted code as a profile that never replaces another", async () => {
        api.ImportCode.mockResolvedValue({profile: {id: "mine", name: "Mine", kind: "custom", apps: [{id: "git"}]}, unknownApps: [], unknownRecipes: []});
        const user = await openApp();
        await go(user, /Profiles/);
        await user.click(screen.getByRole("button", {name: "Import from code…"}));
        const box = await screen.findByRole("textbox", {name: "Paste a share code"});
        await user.type(box, "WF1.xyz");
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Import profile…"}));
        await waitFor(() => expect(api.ImportCode).toHaveBeenCalledWith("WF1.xyz"));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalledWith(expect.objectContaining({id: "mine-2", name: "Mine (2)"})));
    });

    it("says so when the code is not valid", async () => {
        api.ImportCode.mockRejectedValue(new Error("not a WinForge share code"));
        const user = await openApp();
        await go(user, /Profiles/);
        await user.click(screen.getByRole("button", {name: "Import from code…"}));
        await user.type(await screen.findByRole("textbox", {name: "Paste a share code"}), "garbage{Enter}");
        expect(await screen.findByText("not a WinForge share code")).toBeInTheDocument();
        expect(api.SaveProfile).not.toHaveBeenCalled();
    });
});

describe("keyboard", () => {
    it("Ctrl+K opens the catalog search from any screen", async () => {
        const user = await openApp();
        await go(user, /Settings backup/);
        await user.keyboard("{Control>}k{/Control}");
        const box = await screen.findByPlaceholderText(/Search apps/);
        await waitFor(() => expect(box).toHaveFocus());
    });

    it("Ctrl+K does nothing behind an open dialog", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const card = screen.getByRole("button", {name: "Firefox"}).closest("article") as HTMLElement;
        await user.click(within(card).getByRole("button", {name: "Install"}));
        await screen.findByRole("dialog");
        await user.keyboard("{Control>}k{/Control}");
        expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
});
