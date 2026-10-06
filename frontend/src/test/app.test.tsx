import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {healthResult, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const step = (id: string, name: string) => ({kind: "app" as const, id, name});

async function openApp() {
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/); // state loaded: the home screen is up
    return user;
}

async function go(user: ReturnType<typeof userEvent.setup>, tab: RegExp) {
    const nav = screen.getByRole("navigation", {name: "Main menu"});
    await user.click(within(nav).getByRole("button", {name: tab}));
}

const cardOf = (name: string) => screen.getByRole("button", {name}).closest("article") as HTMLElement;

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

describe("first run", () => {
    it("shows the guide once, and never again after it is dismissed", async () => {
        localStorage.removeItem("winforge.tour");
        const user = userEvent.setup();
        const first = render(<App/>);
        const guide = await screen.findByRole("dialog", {name: "Pick what you want"});
        await user.click(within(guide).getByRole("button", {name: "Skip"}));
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
        expect(localStorage.getItem("winforge.tour")).toBe("1");
        first.unmount();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("walks through the three screens and can be reopened from the sidebar", async () => {
        localStorage.removeItem("winforge.tour");
        const user = userEvent.setup();
        render(<App/>);
        await user.click(await screen.findByRole("button", {name: "Next"}));
        await user.click(screen.getByRole("button", {name: "Next"}));
        expect(screen.getByRole("dialog", {name: "Keep your PC healthy"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Let's go"}));
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Show the guide"}));
        expect(await screen.findByRole("dialog", {name: "Pick what you want"})).toBeInTheDocument();
    });
});

describe("installing", () => {
    async function openFirefoxPlan(user: ReturnType<typeof userEvent.setup>) {
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("button", {name: "Install"}));
        return screen.findByRole("dialog", {name: /Install/});
    }

    it("plans, confirms and follows the run to the end, then rescans the PC", async () => {
        const user = await openApp();
        const dialog = await openFirefoxPlan(user);
        expect(api.Plan).toHaveBeenCalledWith(expect.objectContaining({apps: [{id: "firefox"}]}));
        expect(api.Apply).not.toHaveBeenCalled();

        const scans = api.GetState.mock.calls.length;
        await user.click(within(dialog).getByRole("button", {name: "Run 1 steps"}));
        expect(api.Apply).toHaveBeenCalledTimes(1);

        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        expect(await screen.findByText("Installing Firefox…")).toBeInTheDocument();
        await emit.install({step: step("firefox", "Firefox"), status: "output", line: "Downloading 40%"});
        await emit.install({step: step("firefox", "Firefox"), status: "ok"});
        await emit.done([]);

        expect(await screen.findByRole("heading", {name: "Done"})).toBeInTheDocument();
        await waitFor(() => expect(api.GetState.mock.calls.length).toBeGreaterThan(scans));
        await user.click(screen.getByRole("button", {name: "Close"}));
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("starts the run once even if the confirm button is double-clicked", async () => {
        const user = await openApp();
        const dialog = await openFirefoxPlan(user);
        await user.dblClick(within(dialog).getByRole("button", {name: "Run 1 steps"}));
        expect(api.Apply).toHaveBeenCalledTimes(1);
    });

    it("keeps events that arrive before the start call has returned", async () => {
        let release!: () => void;
        api.Apply.mockImplementation(() => new Promise<void>((r) => { release = r; }));
        const user = await openApp();
        const dialog = await openFirefoxPlan(user);
        await user.click(within(dialog).getByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        release();
        await waitFor(() => expect(api.Apply).toHaveBeenCalled());
        expect(await screen.findByText("Installing Firefox…")).toBeInTheDocument();
    });

    it("tells the person when the run cannot start and lets them try again", async () => {
        api.Apply.mockRejectedValueOnce(new Error("an installation is already running"));
        const user = await openApp();
        const dialog = await openFirefoxPlan(user);
        await user.click(within(dialog).getByRole("button", {name: "Run 1 steps"}));
        expect(await screen.findByRole("status")).toHaveTextContent("an installation is already running");
        const again = within(screen.getByRole("dialog")).getByRole("button", {name: "Run 1 steps"});
        expect(again).toBeEnabled();
        await user.click(again);
        expect(api.Apply).toHaveBeenCalledTimes(2);
    });

    it("cannot be dismissed with Escape while running, but can once it has finished", async () => {
        const user = await openApp();
        await openFirefoxPlan(user);
        await user.click(screen.getByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        await user.keyboard("{Escape}");
        expect(screen.getByRole("dialog")).toBeInTheDocument();
        await emit.install({step: step("firefox", "Firefox"), status: "ok"});
        await emit.done([]);
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("stops once: the button disables itself and reports it is stopping", async () => {
        const user = await openApp();
        await openFirefoxPlan(user);
        await user.click(screen.getByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        const stop = screen.getByRole("button", {name: "Stop"});
        await user.dblClick(stop);
        expect(api.Cancel).toHaveBeenCalledTimes(1);
        expect(screen.getByRole("button", {name: "Stopping…"})).toBeDisabled();
    });

    it("shows the failure reason and the right hint when a step fails", async () => {
        const user = await openApp();
        await openFirefoxPlan(user);
        await user.click(screen.getByRole("button", {name: "Run 1 steps"}));
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        await emit.install({step: step("firefox", "Firefox"), status: "failed", error: "winget exited with code 0x80070005"});
        await emit.done(["firefox"]);
        expect(await screen.findByRole("heading", {name: "Finished with errors"})).toBeInTheDocument();
        expect(screen.getByText(/did not finish/)).toBeInTheDocument();
        expect(screen.getByText(/winget exited with code 0x80070005/)).toBeInTheDocument(); // log opens by itself
    });

    it("keeps focus inside the dialog when Tab is pressed repeatedly", async () => {
        const user = await openApp();
        const dialog = await openFirefoxPlan(user);
        for (let i = 0; i < 6; i++) {
            await user.tab();
            expect(dialog).toContainElement(document.activeElement as HTMLElement);
        }
        await user.tab({shift: true});
        expect(dialog).toContainElement(document.activeElement as HTMLElement);
    });

    it("says there is nothing to do instead of opening an empty plan", async () => {
        api.Plan.mockResolvedValueOnce({steps: [], alreadyInstalled: ["firefox"], needsAdmin: false});
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("button", {name: "Install"}));
        expect(await screen.findByRole("status")).toHaveTextContent("Nothing to install");
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
});

describe("profiles", () => {
    async function openMine(user: ReturnType<typeof userEvent.setup>) {
        await go(user, /Profiles/);
        await user.click(await screen.findByRole("button", {name: /Mine/}));
        await screen.findByRole("button", {name: /All profiles/});
    }

    it("asks before deleting, and keeps the profile if the answer is no", async () => {
        const user = await openApp();
        await openMine(user);
        await user.click(screen.getByRole("button", {name: /More options/}));
        await user.click(screen.getByRole("menuitem", {name: "Delete"}));
        const confirm = await screen.findByRole("alertdialog", {name: "Delete “Mine”?"});
        expect(api.DeleteProfile).not.toHaveBeenCalled();
        await user.click(within(confirm).getByRole("button", {name: "Cancel"}));
        expect(api.DeleteProfile).not.toHaveBeenCalled();
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });

    it("deletes after the person confirms", async () => {
        const user = await openApp();
        await openMine(user);
        await user.click(screen.getByRole("button", {name: /More options/}));
        await user.click(screen.getByRole("menuitem", {name: "Delete"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Delete"}));
        expect(api.DeleteProfile).toHaveBeenCalledWith("mine");
    });

    it("focuses the safe answer when asking to delete", async () => {
        const user = await openApp();
        await openMine(user);
        await user.click(screen.getByRole("button", {name: /More options/}));
        await user.click(screen.getByRole("menuitem", {name: "Delete"}));
        const confirm = await screen.findByRole("alertdialog");
        expect(within(confirm).getByRole("button", {name: "Cancel"})).toHaveFocus();
    });

    it("closes the options menu on outside click and Escape", async () => {
        const user = await openApp();
        await openMine(user);
        await user.click(screen.getByRole("button", {name: /More options/}));
        expect(screen.getByRole("menu")).toBeInTheDocument();
        await user.click(screen.getByRole("heading", {name: "Mine"}));
        expect(screen.queryByRole("menu")).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: /More options/}));
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    });

    it("never replaces an existing profile: a saved selection gets a free id", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("checkbox", {name: "Firefox"}));
        await user.click(screen.getByRole("button", {name: "Save as profile…"}));
        const name = within(await screen.findByRole("dialog", {name: "Save selection as a profile"})).getByRole("textbox");
        await user.type(name, "Gaming{Enter}"); // "gaming" is a built-in id
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalled());
        expect(api.SaveProfile).toHaveBeenCalledWith(expect.objectContaining({id: "gaming-2", name: "Gaming (2)", apps: [{id: "firefox"}]}));
    });

    it("renames an imported profile instead of overwriting one that exists", async () => {
        api.ImportProfile.mockResolvedValue({profile: {id: "mine", name: "Mine", apps: [{id: "gimp"}]}, unknownApps: [], unknownRecipes: []});
        const user = await openApp();
        await go(user, /Profiles/);
        await user.click(screen.getByRole("button", {name: "Import profile…"}));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalled());
        expect(api.SaveProfile).toHaveBeenCalledWith(expect.objectContaining({id: "mine-2", name: "Mine (2)"}));
    });

    it("installs only the apps that stay ticked", async () => {
        const user = await openApp();
        await go(user, /Profiles/);
        await user.click(await screen.findByRole("button", {name: /Gaming/}));
        await user.click(screen.getByRole("checkbox", {name: "Include Firefox"}));
        // Firefox unticked, VLC is already installed: nothing is left to install.
        expect(screen.getByRole("button", {name: /Install 0 selected/})).toBeDisabled();
    });
});

describe("updates", () => {
    const ups = [
        {id: "git", name: "Git", publisher: "Vendor", current: "1.0", available: "2.0"},
        {id: "firefox", name: "Firefox", publisher: "Vendor", current: "5", available: "6"},
    ];

    it("lists what can be updated with everything ticked, and updates all with an empty selection", async () => {
        api.Upgrades.mockResolvedValue(ups);
        api.PlanUpgrades.mockResolvedValue({steps: [{kind: "upgrade", id: "git", name: "Git"}], alreadyInstalled: [], needsAdmin: false});
        const user = await openApp();
        await go(user, /Updates/);
        expect(await screen.findByRole("checkbox", {name: "Git"})).toBeChecked();
        await user.click(screen.getByRole("button", {name: "Update all"}));
        expect(api.PlanUpgrades).toHaveBeenCalledWith([]);
    });

    it("updates only the ticked apps", async () => {
        api.Upgrades.mockResolvedValue(ups);
        api.PlanUpgrades.mockResolvedValue({steps: [{kind: "upgrade", id: "firefox", name: "Firefox"}], alreadyInstalled: [], needsAdmin: false});
        const user = await openApp();
        await go(user, /Updates/);
        await user.click(await screen.findByRole("checkbox", {name: "Git"}));
        await user.click(screen.getByRole("button", {name: "Update 1…"}));
        expect(api.PlanUpgrades).toHaveBeenCalledWith(["firefox"]);
    });

    it("says so when everything is current", async () => {
        const user = await openApp();
        await go(user, /Updates/);
        expect(await screen.findByText("Everything from the catalog is up to date.")).toBeInTheDocument();
    });

    it("reports a failed check instead of spinning forever", async () => {
        api.Upgrades.mockRejectedValue(new Error("winget is not available"));
        const user = await openApp();
        await go(user, /Updates/);
        expect(await screen.findByRole("status")).toHaveTextContent("winget is not available");
        expect(screen.getByRole("button", {name: "Check for updates"})).toBeEnabled();
    });
});

describe("PC health", () => {
    it("shows the score, what needs fixing and opens vendor links", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const user = await openApp();
        await go(user, /PC health/);
        expect(await screen.findByRole("heading", {name: "Something needs fixing"})).toBeInTheDocument();
        expect(screen.getByText("2 devices need a driver")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: /ASUS support/}));
        expect(api.OpenLink).toHaveBeenCalledWith("https://www.asus.com/support/");
    });

    it("offers to restart as administrator when checks could not be answered", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const user = await openApp();
        await go(user, /PC health/);
        await user.click(await screen.findByRole("button", {name: "Restart as administrator"}));
        expect(api.RestartAsAdmin).toHaveBeenCalled();
    });

    it("exports a report that carries the findings", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const user = await openApp();
        await go(user, /PC health/);
        await user.click(await screen.findByRole("button", {name: "Export report"}));
        await waitFor(() => expect(api.SaveTextFile).toHaveBeenCalled());
        const [name, content] = api.SaveTextFile.mock.calls[0];
        expect(name).toBe("winforge-pc-health.md");
        expect(content).toContain("# PC health");
        expect(content).toContain("2 devices need a driver");
        expect(await screen.findByRole("status")).toHaveTextContent("Report saved to C:\\report.md");
    });

    it("tells the person when the scan fails, and lets them retry", async () => {
        api.HealthScan.mockRejectedValueOnce(new Error("timed out after 2m0s"));
        const user = await openApp();
        await go(user, /PC health/);
        expect(await screen.findByRole("status")).toHaveTextContent("timed out");
        api.HealthScan.mockResolvedValue(healthResult());
        await user.click(screen.getByRole("button", {name: "Scan"}));
        expect(await screen.findByText("2 devices need a driver")).toBeInTheDocument();
    });

    it("keeps the driver table out of sight until asked, and opens it in advanced mode", async () => {
        api.HealthScan.mockResolvedValue(healthResult());
        const user = await openApp();
        await go(user, /PC health/);
        await screen.findByText("2 devices need a driver");
        expect(screen.queryByRole("table")).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Advanced"}));
        expect(await screen.findByRole("table")).toBeInTheDocument();
        expect(screen.getByText("Realtek Audio")).toBeInTheDocument();
    });
});

describe("settings and language", () => {
    it("switches the whole interface to Spanish and remembers it", async () => {
        const user = await openApp();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Listo"}));
        const nav = screen.getByRole("navigation", {name: "Menú principal"});
        expect(within(nav).getByRole("button", {name: /Inicio/})).toBeInTheDocument();
        expect(JSON.parse(localStorage.getItem("winforge.settings") ?? "{}").language).toBe("es");
        expect(document.documentElement.lang).toBe("es");
    });

    it("closes with Escape and gives focus back to the button that opened it", async () => {
        const user = await openApp();
        const opener = screen.getByRole("button", {name: "Appearance and language"});
        await user.click(opener);
        expect(screen.getByRole("dialog")).toBeInTheDocument();
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
        expect(opener).toHaveFocus();
    });

    it("advanced mode reveals technical details the simple mode hides", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("button", {name: "Firefox"}));
        const drawer = await screen.findByRole("dialog", {name: "Firefox"});
        expect(within(drawer).queryByText("Vendor.firefox")).not.toBeInTheDocument();
        await user.click(within(drawer).getByRole("button", {name: "Close"}));
        await user.click(screen.getByRole("button", {name: "Advanced"}));
        await user.click(within(cardOf("Firefox")).getByRole("button", {name: "Firefox"}));
        expect(within(await screen.findByRole("dialog", {name: "Firefox"})).getByText("Vendor.firefox")).toBeInTheDocument();
    });
});

describe("app detail panel", () => {
    it("closes with Escape without closing anything else, and returns focus", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        const open = within(cardOf("Firefox")).getByRole("button", {name: "Firefox"});
        await user.click(open);
        expect(await screen.findByRole("dialog", {name: "Firefox"})).toBeInTheDocument();
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
        expect(open).toHaveFocus();
        expect(screen.getByRole("navigation", {name: "Main menu"})).toBeInTheDocument();
    });

    it("opens the website through the app, not the webview", async () => {
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("button", {name: "Firefox"}));
        await user.click(await screen.findByRole("button", {name: /Website/}));
        expect(api.OpenURL).toHaveBeenCalledWith("https://example.com");
    });
});

describe("robustness", () => {
    it("turns an unhandled promise rejection into a message", async () => {
        await openApp();
        const e = new Event("unhandledrejection", {cancelable: true}) as Event & {reason?: unknown};
        e.reason = new Error("boom from nowhere");
        window.dispatchEvent(e);
        expect(await screen.findByRole("status")).toHaveTextContent("boom from nowhere");
        expect(e.defaultPrevented).toBe(true);
    });

    it("survives the PC scan itself failing at startup", async () => {
        api.GetState.mockRejectedValueOnce(new Error("catalog unavailable"));
        render(<App/>);
        expect(await screen.findByRole("status")).toHaveTextContent("catalog unavailable");
    });

    it("shows a friendly screen, not a blank window, if rendering crashes", async () => {
        const {ErrorBoundary} = await import("../components/ErrorBoundary");
        const Bomb = () => { throw new Error("render exploded"); };
        const spy = vi.spyOn(console, "error").mockImplementation(() => {});
        render(<ErrorBoundary><Bomb/></ErrorBoundary>);
        expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
        expect(screen.getByText("render exploded")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Reload WinForge"})).toBeInTheDocument();
        spy.mockRestore();
    });

    it("a selection survives a rescan and still counts only what is missing", async () => {
        const state = makeState();
        resetApi(state);
        const user = await openApp();
        await go(user, /Catalog/);
        await user.click(within(cardOf("Firefox")).getByRole("checkbox", {name: "Firefox"}));
        await user.click(screen.getByRole("button", {name: "Rescan PC"}));
        await waitFor(() => expect(api.GetState.mock.calls.length).toBeGreaterThan(1));
        expect(screen.getByText(/1 selected, 1 to install/)).toBeInTheDocument();
    });
});
