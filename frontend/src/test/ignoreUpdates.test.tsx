import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import type {UpgradeInfo} from "../lib/model";
import {api, resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const up = (id: string, name: string, extra: Partial<UpgradeInfo> = {}): UpgradeInfo =>
    ({id, name, publisher: "Vendor", current: "1.0", available: "2.0", ...extra});
const GIT = up("git", "Git");
const FIREFOX = up("firefox", "Firefox");

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
    api.Upgrades.mockResolvedValue([GIT, FIREFOX]);
});

async function openUpdates() {
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Updates/}));
    await screen.findByRole("checkbox", {name: "Include Git in the update"});
    return user;
}

const rowOf = (name: string) => screen.getByRole("checkbox", {name: `Include ${name} in the update`}).closest("li") as HTMLElement;

describe("ignoring updates", () => {
    it("offers skip, pause and never for each update, with the version spelled out", async () => {
        const user = await openUpdates();
        await user.click(within(rowOf("Git")).getByRole("button", {name: "Ignore options for Git"}));
        const menu = screen.getByRole("menu");
        expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
            "Skip version 2.0", "Pause for 7 days", "Pause for 30 days", "Pause for 90 days", "Never update this app",
        ]);
    });

    it("hides an app that is ignored forever, tells the person, and keeps it out of the counts", async () => {
        api.IgnoreUpdate.mockResolvedValue([up("git", "Git", {ignore: "always"}), FIREFOX]);
        const user = await openUpdates();
        const nav = within(screen.getByRole("navigation", {name: "Main menu"}));
        expect(nav.getByRole("button", {name: /Updates/})).toHaveTextContent("2");
        await user.click(within(rowOf("Git")).getByRole("button", {name: "Ignore options for Git"}));
        await user.click(screen.getByRole("menuitem", {name: "Never update this app"}));
        expect(api.IgnoreUpdate).toHaveBeenCalledWith("git", "always", 0);
        expect(await screen.findByText(/Git: update hidden/)).toBeInTheDocument();
        expect(screen.queryByRole("checkbox", {name: "Include Git in the update"})).not.toBeInTheDocument();
        expect(nav.getByRole("button", {name: /Updates/})).toHaveTextContent("1");
        // the button now updates only what is left
        expect(screen.getByRole("button", {name: "Update all"})).toBeEnabled();
    });

    it("sends the chosen way and length to the backend", async () => {
        api.IgnoreUpdate.mockResolvedValue([GIT, FIREFOX]); // the list is not what this test is about
        const user = await openUpdates();
        await user.click(within(rowOf("Git")).getByRole("button", {name: "Ignore options for Git"}));
        await user.click(screen.getByRole("menuitem", {name: "Pause for 30 days"}));
        expect(api.IgnoreUpdate).toHaveBeenLastCalledWith("git", "until", 30);
        await user.click(within(rowOf("Firefox")).getByRole("button", {name: "Ignore options for Firefox"}));
        await user.click(screen.getByRole("menuitem", {name: "Skip version 2.0"}));
        expect(api.IgnoreUpdate).toHaveBeenLastCalledWith("firefox", "version", 0);
    });

    it("lists ignored updates apart, says why, and brings one back", async () => {
        api.Upgrades.mockResolvedValue([
            GIT,
            up("firefox", "Firefox", {ignore: "version", ignoreVersion: "2.0"}),
            up("vlc", "VLC", {ignore: "until", ignoreUntil: "2026-12-24T10:00:00Z"}),
            up("7zip", "7-Zip", {ignore: "always"}),
        ]);
        api.StopIgnoringUpdate.mockResolvedValue([GIT, FIREFOX]);
        const user = await openUpdates();
        const toggle = screen.getByRole("button", {name: /Ignored updates \(3\)/});
        expect(toggle).toHaveAttribute("aria-expanded", "false");
        expect(screen.queryByText("skipping version 2.0")).not.toBeInTheDocument();
        await user.click(toggle);
        expect(toggle).toHaveAttribute("aria-expanded", "true");
        expect(screen.getByText("skipping version 2.0")).toBeInTheDocument();
        expect(screen.getByText(/paused until .*2026/)).toBeInTheDocument();
        expect(screen.getByText("never updated")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Stop ignoring updates of Firefox"}));
        expect(api.StopIgnoringUpdate).toHaveBeenCalledWith("firefox");
        expect(await screen.findByRole("checkbox", {name: "Include Firefox in the update"})).toBeChecked();
        expect(await screen.findByText(/Firefox: update shown again/)).toBeInTheDocument();
    });

    it("shows what the backend refused, and changes nothing on screen", async () => {
        api.IgnoreUpdate.mockRejectedValue(new Error("a pause lasts between 1 and 365 days"));
        const user = await openUpdates();
        await user.click(within(rowOf("Git")).getByRole("button", {name: "Ignore options for Git"}));
        await user.click(screen.getByRole("menuitem", {name: "Pause for 7 days"}));
        expect(await screen.findByText("a pause lasts between 1 and 365 days")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Include Git in the update"})).toBeInTheDocument();
    });

    it("works by keyboard: Enter opens the menu on its first item, arrows move, Escape returns to the button", async () => {
        const user = await openUpdates();
        const trigger = within(rowOf("Git")).getByRole("button", {name: "Ignore options for Git"});
        trigger.focus();
        await user.keyboard("{Enter}");
        const items = screen.getAllByRole("menuitem");
        await waitFor(() => expect(items[0]).toHaveFocus());
        await user.keyboard("{ArrowDown}{ArrowDown}");
        expect(items[2]).toHaveFocus();
        await user.keyboard("{ArrowUp}{ArrowUp}{ArrowUp}"); // wraps to the last
        expect(items[items.length - 1]).toHaveFocus();
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("menu")).not.toBeInTheDocument();
        expect(trigger).toHaveFocus();
        expect(trigger).toHaveAttribute("aria-expanded", "false");
    });

    it("says everything is ignored instead of showing an empty list as if all were up to date", async () => {
        api.Upgrades.mockResolvedValue([up("git", "Git", {ignore: "always"})]);
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Updates/}));
        expect(await screen.findByText("Everything from the catalog is up to date.")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: /Ignored updates \(1\)/})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Update 0…"})).toBeDisabled(); // nothing left to update
    });

    it("speaks Spanish too", async () => {
        const user = userEvent.setup();
        api.Upgrades.mockResolvedValue([GIT, up("vlc", "VLC", {ignore: "always"})]);
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Listo"}));
        await user.click(within(screen.getByRole("navigation", {name: "Menú principal"})).getByRole("button", {name: /Actualizaciones/}));
        await user.click(await screen.findByRole("button", {name: "Opciones para ignorar Git"}));
        expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
            "Saltar la versión 2.0", "Pausar 7 días", "Pausar 30 días", "Pausar 90 días", "No actualizar nunca esta app",
        ]);
        await user.keyboard("{Escape}");
        expect(screen.getByRole("button", {name: /Actualizaciones ignoradas \(1\)/})).toBeInTheDocument();
    });
});
