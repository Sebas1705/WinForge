import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {DriftInfo} from "../api";
import App from "../App";
import {api, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const none: DriftInfo = {hasBaseline: false, removed: [], added: [], updated: [], count: 0};
const changed: DriftInfo = {
    hasBaseline: true, at: "2026-11-01T10:00:00Z", count: 4,
    removed: [{id: "vlc", name: "VLC"}],
    added: [{id: "git", name: "Git"}, {id: "firefox", name: "Firefox"}],
    updated: [{id: "gimp", name: "GIMP", from: "2.9", to: "3.0"}],
};

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

async function open(profile: RegExp = /Mine/, language: "en" | "es" = "en") {
    if (language === "es") localStorage.setItem("winforge.settings", JSON.stringify({language: "es"}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(language === "es" ? /apps del catálogo encontradas/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: language === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: language === "es" ? /Perfiles/ : /Profiles/}));
    await user.click(await screen.findByRole("button", {name: profile}));
    return user;
}

describe("what changed since a profile was applied", () => {
    it("says there is no record, and offers to remember this PC as it is", async () => {
        api.ProfileDrift.mockResolvedValueOnce(none).mockResolvedValue({hasBaseline: true, at: "2026-11-15T09:00:00Z", count: 0, removed: [], added: [], updated: []});
        const user = await open();
        expect(await screen.findByText("WinForge has no record of when this profile was applied.")).toBeInTheDocument();
        expect(api.ProfileDrift).toHaveBeenCalledWith("mine");
        const button = screen.getByRole("button", {name: "Remember this PC as it is now"});
        expect(button).toHaveAttribute("title", expect.stringContaining("Nothing on the PC changes"));
        await user.click(button);
        expect(api.AcceptProfileState).toHaveBeenCalledWith("mine");
        expect(await screen.findByText(/“Mine” now matches this PC\./)).toBeInTheDocument();
        expect(await screen.findByText(/In step with this profile since/)).toBeInTheDocument(); // read again
        expect(screen.queryByText("WinForge has no record of when this profile was applied.")).not.toBeInTheDocument();
    });

    it("says the PC is in step when nothing changed, and shows nothing more", async () => {
        api.ProfileDrift.mockResolvedValue({hasBaseline: true, at: "2026-11-01T10:00:00Z", count: 0, removed: [], added: [], updated: []});
        await open();
        expect(await screen.findByText(/In step with this profile since .*2026\./)).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Accept these changes"})).not.toBeInTheDocument();
    });

    it("lists what was removed, what was added and which versions moved", async () => {
        api.ProfileDrift.mockResolvedValue(changed);
        await open();
        const panel = (await screen.findByRole("heading", {name: /Changed since .*2026/})).closest("section") as HTMLElement;
        const inside = within(panel);
        expect(inside.getByRole("heading", {name: "No longer installed"}).parentElement).toHaveTextContent("VLC");
        expect(inside.getByRole("heading", {name: "Installed since, not in this profile"}).parentElement).toHaveTextContent(/Git.*Firefox/);
        const versions = inside.getByRole("heading", {name: "Versions that changed"}).parentElement as HTMLElement;
        expect(versions).toHaveTextContent("GIMP");
        expect(versions).toHaveTextContent("2.9 → 3.0");
        expect(inside.getByText("Install what is missing puts them back.")).toBeInTheDocument();
    });

    it("shows only the sections that have something in them", async () => {
        api.ProfileDrift.mockResolvedValue({hasBaseline: true, at: "2026-11-01T10:00:00Z", count: 1, removed: [{id: "vlc", name: "VLC"}], added: [], updated: []});
        await open();
        await screen.findByRole("heading", {name: "No longer installed"});
        expect(screen.queryByRole("heading", {name: "Installed since, not in this profile"})).not.toBeInTheDocument();
        expect(screen.queryByRole("heading", {name: "Versions that changed"})).not.toBeInTheDocument();
    });

    it("accepts the changes without touching the PC, then reads the report again", async () => {
        api.ProfileDrift.mockResolvedValueOnce(changed).mockResolvedValue({hasBaseline: true, at: "2026-11-15T09:00:00Z", count: 0, removed: [], added: [], updated: []});
        const user = await open();
        const accept = await screen.findByRole("button", {name: "Accept these changes"});
        expect(accept).toHaveAttribute("title", expect.stringContaining("Nothing on the PC changes"));
        await user.click(accept);
        expect(api.AcceptProfileState).toHaveBeenCalledWith("mine");
        expect(api.Apply).not.toHaveBeenCalled();
        expect(api.SaveProfile).not.toHaveBeenCalled();
        expect(await screen.findByText(/In step with this profile since/)).toBeInTheDocument();
    });

    it("adds the new apps to the person's own profile, without leaving the page", async () => {
        api.ProfileDrift.mockResolvedValueOnce(changed).mockResolvedValue({hasBaseline: true, at: "2026-11-01T10:00:00Z", count: 2, removed: [], added: [], updated: []});
        const user = await open();
        const getStates = api.GetState.mock.calls.length;
        await user.click(await screen.findByRole("button", {name: "Add 2 to this profile"}));
        expect(api.SaveProfile).toHaveBeenCalledTimes(1);
        const saved = api.SaveProfile.mock.calls[0][0];
        expect(saved.id).toBe("mine");
        expect(saved.apps).toEqual([{id: "gimp"}, {id: "firefox"}, {id: "git"}]); // what was there, then the new ones sorted
        expect(Object.keys(saved)).not.toContain("resolved"); // a profile, not what the interface built from it
        expect(await screen.findByText("2 apps added to “Mine”.")).toBeInTheDocument();
        await waitFor(() => expect(api.GetState.mock.calls.length).toBeGreaterThan(getStates)); // profiles read again
        expect(screen.getByRole("heading", {name: "Mine"})).toBeInTheDocument(); // still on the profile
    });

    it("does not offer to edit a built-in profile, and says why", async () => {
        api.ProfileDrift.mockResolvedValue(changed);
        await open(/Gaming/);
        await screen.findByRole("heading", {name: /Changed since/});
        expect(screen.queryByRole("button", {name: /to this profile/})).not.toBeInTheDocument();
        expect(screen.getByText(/Built-in profiles cannot be changed/)).toBeInTheDocument();
    });

    it("reports a failure to check, and a failure to save, without losing the page", async () => {
        api.ProfileDrift.mockRejectedValueOnce(new Error("this PC has not been scanned yet"));
        await open();
        expect(await screen.findByRole("note")).toHaveTextContent("Could not check for changes: this PC has not been scanned yet");
    });

    it("shows the error when adding the apps fails", async () => {
        api.ProfileDrift.mockResolvedValue(changed);
        api.SaveProfile.mockRejectedValue(new Error("disk is full"));
        const user = await open();
        await user.click(await screen.findByRole("button", {name: "Add 2 to this profile"}));
        expect(await screen.findByText("disk is full")).toBeInTheDocument();
        expect(screen.queryByText(/apps added to/)).not.toBeInTheDocument();
    });

    it("reads the report again when the PC is scanned again", async () => {
        api.ProfileDrift.mockResolvedValue(none);
        const user = await open();
        await screen.findByText("WinForge has no record of when this profile was applied.");
        const before = api.ProfileDrift.mock.calls.length;
        await user.click(screen.getByRole("button", {name: "Rescan PC"}));
        await waitFor(() => expect(api.ProfileDrift.mock.calls.length).toBeGreaterThan(before));
    });

    it("speaks Spanish", async () => {
        api.ProfileDrift.mockResolvedValue(changed);
        await open(/Mine/, "es");
        expect(await screen.findByRole("heading", {name: /Cambios desde el .*2026/})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Ya no están instaladas"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Añadir 2 a este perfil"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Aceptar estos cambios"})).toBeInTheDocument();
    });
});

describe("the change count on the profile cards", () => {
    it("shows how many things changed, in the singular too, and nothing when none", async () => {
        const state = makeState();
        state.profiles = state.profiles.map((p) => ({...p, changes: p.id === "gaming" ? 3 : p.id === "mine" ? 1 : 0}));
        api.GetState.mockResolvedValue(state);
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Profiles/}));
        const card = (name: RegExp) => screen.getByRole("button", {name}) as HTMLElement;
        expect(within(card(/Gaming/)).getByText("3 changes")).toBeInTheDocument();
        expect(within(card(/Mine/)).getByText("1 change")).toBeInTheDocument();
        const badges = [...document.querySelectorAll(".pill.changed")].map((b) => b.textContent);
        expect(badges).toEqual(["3 changes", "1 change"]); // the profile with none shows no badge
    });
});
