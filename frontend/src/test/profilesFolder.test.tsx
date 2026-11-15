import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {ProfilesFolderInfo} from "../api";
import App from "../App";
import {api, resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const DEFAULT = "C:\\Users\\me\\AppData\\Roaming\\WinForge\\profiles";
const SYNCED = "D:\\OneDrive\\WinForge profiles";
const info = (extra: Partial<ProfilesFolderInfo> = {}): ProfilesFolderInfo => ({path: DEFAULT, default: DEFAULT, custom: false, count: 3, ...extra});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
    api.ProfilesFolder.mockResolvedValue(info());
});

async function openSettings() {
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(screen.getByRole("button", {name: "Appearance and language"}));
    await screen.findByText(DEFAULT);
    return user;
}

describe("the profiles folder in the settings", () => {
    it("shows where profiles are kept and how many are there", async () => {
        await openSettings();
        const dialog = within(screen.getByRole("dialog"));
        expect(dialog.getByText("Profiles folder")).toBeInTheDocument();
        expect(dialog.getByText(DEFAULT)).toBeInTheDocument();
        expect(dialog.getByText("3 profiles here")).toBeInTheDocument();
        expect(dialog.getByText(/OneDrive or Dropbox keeps in sync/)).toBeInTheDocument();
        expect(dialog.queryByRole("button", {name: "Use the default"})).not.toBeInTheDocument(); // already the default
    });

    it("asks before moving, offers to copy the profiles, and moves them", async () => {
        api.PickFolder.mockResolvedValue(SYNCED);
        api.SetProfilesFolder.mockResolvedValue({copied: 3, skipped: 0, info: info({path: SYNCED, custom: true})});
        const user = await openSettings();
        const getStates = api.GetState.mock.calls.length;
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Change…"}));
        const ask = await screen.findByRole("alertdialog", {name: "Keep profiles in this folder?"});
        expect(ask).toHaveTextContent(SYNCED);
        expect(ask).toHaveTextContent("The folder they are in now is left as it is.");
        const copy = within(ask).getByRole("checkbox", {name: "Copy my 3 profiles there first (files already there are kept)"});
        expect(copy).toBeChecked();
        expect(api.SetProfilesFolder).not.toHaveBeenCalled(); // nothing moves until it is confirmed
        await user.click(within(ask).getByRole("button", {name: "Use this folder"}));
        expect(api.SetProfilesFolder).toHaveBeenCalledWith(SYNCED, true);
        expect(await screen.findByText(/Profiles are now kept in D:\\OneDrive\\WinForge profiles\. 3 copied, 0 already there\./)).toBeInTheDocument();
        await waitFor(() => expect(api.GetState.mock.calls.length).toBeGreaterThan(getStates)); // the profile list is read again
        expect(within(screen.getByRole("dialog")).getByText(SYNCED)).toBeInTheDocument();
        expect(within(screen.getByRole("dialog")).getByRole("button", {name: "Use the default"})).toBeInTheDocument();
    });

    it("can move without copying", async () => {
        api.PickFolder.mockResolvedValue(SYNCED);
        api.SetProfilesFolder.mockResolvedValue({copied: 0, skipped: 0, info: info({path: SYNCED, custom: true})});
        const user = await openSettings();
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Change…"}));
        const ask = await screen.findByRole("alertdialog");
        await user.click(within(ask).getByRole("checkbox"));
        await user.click(within(ask).getByRole("button", {name: "Use this folder"}));
        expect(api.SetProfilesFolder).toHaveBeenCalledWith(SYNCED, false);
        const toast = await screen.findByText(/Profiles are now kept in/);
        expect(toast).not.toHaveTextContent("copied");
    });

    it("does nothing when the folder picker is cancelled or the question is declined", async () => {
        const user = await openSettings();
        api.PickFolder.mockResolvedValueOnce("");
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Change…"}));
        await waitFor(() => expect(api.PickFolder).toHaveBeenCalledTimes(1));
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        api.PickFolder.mockResolvedValueOnce(SYNCED);
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Change…"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Cancel"}));
        expect(api.SetProfilesFolder).not.toHaveBeenCalled();
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(within(screen.getByRole("dialog")).getByText(DEFAULT)).toBeInTheDocument();
    });

    it("shows what the backend refused and keeps the old folder on screen", async () => {
        api.PickFolder.mockResolvedValue("D:\\gone");
        api.SetProfilesFolder.mockRejectedValue(new Error("D:\\gone is not available: the system cannot find the path"));
        const user = await openSettings();
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Change…"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Use this folder"}));
        expect(await screen.findByText(/D:\\gone is not available/)).toBeInTheDocument();
        expect(within(screen.getByRole("dialog")).getByText(DEFAULT)).toBeInTheDocument();
    });

    it("goes back to the default folder", async () => {
        api.ProfilesFolder.mockResolvedValue(info({path: SYNCED, custom: true}));
        api.ResetProfilesFolder.mockResolvedValue(info());
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await screen.findByText(SYNCED);
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Use the default"}));
        expect(api.ResetProfilesFolder).toHaveBeenCalledTimes(1);
        expect(await screen.findByText("Profiles are back in the default folder.")).toBeInTheDocument();
        expect(within(screen.getByRole("dialog")).getByText(DEFAULT)).toBeInTheDocument();
    });

    it("explains a folder that is not available, instead of failing quietly", async () => {
        api.ProfilesFolder.mockResolvedValue(info({problem: "The profiles folder is not available (D:\\OneDrive). WinForge is using the default one until it is."}));
        await openSettings();
        expect(within(screen.getByRole("dialog")).getByRole("note")).toHaveTextContent(/not available.*default one/);
    });

    it("speaks Spanish too", async () => {
        api.PickFolder.mockResolvedValue(SYNCED);
        const user = await openSettings();
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        const dialog = within(screen.getByRole("dialog"));
        expect(dialog.getByText("Carpeta de perfiles")).toBeInTheDocument();
        expect(dialog.getByText("3 perfiles aquí")).toBeInTheDocument();
        await user.click(dialog.getByRole("button", {name: "Cambiar…"}));
        const ask = await screen.findByRole("alertdialog", {name: "¿Guardar los perfiles en esta carpeta?"});
        expect(within(ask).getByRole("checkbox", {name: /Copiar antes mis 3 perfiles/})).toBeChecked();
        expect(within(ask).getByRole("button", {name: "Usar esta carpeta"})).toBeInTheDocument();
    });
});
