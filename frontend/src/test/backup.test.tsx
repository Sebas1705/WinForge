import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {BackupPreview} from "../api";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const step = (id: string, name: string) => ({kind: "app" as const, id, name});

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

const preview = (o: Partial<BackupPreview> = {}): BackupPreview => ({
    host: "PC-VIEJO", app: "v0.6.0", createdAt: "2026-10-01 10:00", intact: true, checked: true, problems: [],
    sets: [{id: "vscode", files: 4, new: 3, changed: 1, same: 0}, {id: "git", files: 1, new: 0, changed: 0, same: 1}],
    apps: [{id: "firefox", name: "Firefox", installed: false}, {id: "vlc", name: "VLC", installed: true}], missing: 1, unknown: [],
    profile: {id: "backup", name: "Backup PC-VIEJO", kind: "custom", apps: [{id: "firefox"}, {id: "vlc"}]},
    folders: [{name: "Proyectos", dir: "folders/1", files: 7, bytes: 4096, skipped: 0}], ...o,
});

describe("creating a backup", () => {
    it("lists what exists and creates a verified backup of what is ticked", async () => {
        api.BackupSets.mockResolvedValue([{id: "vscode", files: 4, bytes: 2048}, {id: "git", files: 1, bytes: 90}]);
        api.CreateBackup.mockResolvedValue({path: "C:\\b.zip", files: 5, bytes: 2138, apps: 1, folders: 0, verified: true, problems: 0});
        const user = await openApp();
        await go(user, /Settings backup/);
        await screen.findByText("Visual Studio Code");
        await user.click(screen.getByRole("checkbox", {name: "Git"}));
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        await waitFor(() => expect(api.CreateBackup).toHaveBeenCalledWith({sets: ["vscode"], apps: true, folders: []}));
        expect(await screen.findByText(/Backup saved to C:\\b.zip: 5 files, 2 KB\. Read back and checked/)).toBeInTheDocument();
    });

    it("says so when the saved file does not pass its own check", async () => {
        api.BackupSets.mockResolvedValue([{id: "git", files: 1, bytes: 90}]);
        api.CreateBackup.mockResolvedValue({path: "C:\\b.zip", files: 1, bytes: 90, apps: 0, folders: 0, verified: false, problems: 2});
        const user = await openApp();
        await go(user, /Settings backup/);
        await screen.findByText("Git");
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        expect(await screen.findByText(/did not pass its own check \(2 problems\)/)).toBeInTheDocument();
    });

    it("adds and removes the folders of the user", async () => {
        api.PickFolder.mockResolvedValueOnce("D:\\Proyectos").mockResolvedValueOnce("D:\\Proyectos").mockResolvedValueOnce("E:\\Fotos");
        api.CreateBackup.mockResolvedValue({path: "", files: 0, bytes: 0, apps: 0, folders: 0, verified: false, problems: 0});
        const user = await openApp();
        await go(user, /Settings backup/);
        await user.click(await screen.findByRole("button", {name: "Add a folder…"}));
        await user.click(screen.getByRole("button", {name: "Add a folder…"})); // the same folder twice is one folder
        await user.click(screen.getByRole("button", {name: "Add a folder…"}));
        expect(screen.getByRole("button", {name: "Remove Proyectos"})).toBeInTheDocument();
        expect(screen.getAllByRole("button", {name: /^Remove /}).length).toBe(2);
        await user.click(screen.getByRole("button", {name: "Remove Fotos"}));
        await user.click(screen.getByRole("button", {name: "Create backup…"}));
        await waitFor(() => expect(api.CreateBackup).toHaveBeenCalledWith(expect.objectContaining({folders: ["D:\\Proyectos"]})));
    });

    it("will not create an empty backup", async () => {
        const user = await openApp(makeState({apps: []}));
        await go(user, /Settings backup/);
        await user.click(await screen.findByRole("button", {name: "Create backup…"}));
        expect(await screen.findByText("Pick at least one thing to include.")).toBeInTheDocument();
        expect(api.CreateBackup).not.toHaveBeenCalled();
    });
});

describe("rebuilding from a backup", () => {
    async function open(p = preview()) {
        api.PickBackup.mockResolvedValue(p);
        const user = await openApp();
        await go(user, /Settings backup/);
        await user.click(await screen.findByRole("button", {name: "Rebuild from a backup"}));
        await user.click(screen.getByRole("button", {name: "Choose a backup…"}));
        await screen.findByText(/From PC-VIEJO/);
        return user;
    }

    it("shows where it is from, that it is intact, and what each stage would do", async () => {
        await open();
        expect(screen.getByText("Checked: nothing was altered or damaged.")).toBeInTheDocument();
        expect(screen.getByText("1 to install, 1 already on this PC.")).toBeInTheDocument();
        expect(screen.getByText("3 new · 1 changed · 0 same")).toBeInTheDocument();
        expect(screen.getByRole("checkbox", {name: "Visual Studio Code"})).toBeChecked();
        expect(screen.getByRole("checkbox", {name: "Git"})).not.toBeChecked(); // nothing to change
    });

    it("warns when files were altered or damaged", async () => {
        await open(preview({intact: false, problems: ["git/home/.gitconfig", "profile.json"]}));
        expect(screen.getByRole("alert")).toHaveTextContent("2 files are damaged or were changed");
    });

    it("says so for an old backup that cannot be checked", async () => {
        await open(preview({checked: false}));
        expect(screen.getByText("An older backup without checksums, so it cannot be checked.")).toBeInTheDocument();
    });

    it("installs the apps first, and restores settings and folders only after that dialog closes", async () => {
        api.RestoreSettings.mockResolvedValue({restored: 4, unchanged: 0, backedUp: 0, extensions: 2, skipped: []});
        api.RestoreFolders.mockResolvedValue({written: 7, same: 0, kept: 0, skipped: [], dest: "D:\\Nuevo"});
        api.PickFolder.mockResolvedValue("D:\\Nuevo");
        api.Plan.mockResolvedValue({steps: [step("firefox", "Firefox")], alreadyInstalled: [], needsAdmin: false});
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Choose a folder…"}));
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));

        await user.click(await screen.findByRole("button", {name: "Run 1 steps"}));
        expect(api.Apply).toHaveBeenCalledWith(expect.objectContaining({id: "backup"}));
        expect(api.RestoreSettings).not.toHaveBeenCalled();
        await emit.install({step: step("firefox", "Firefox"), status: "start"});
        await emit.install({step: step("firefox", "Firefox"), status: "ok"});
        await emit.done([]);
        expect(api.RestoreSettings).not.toHaveBeenCalled(); // still waiting for the dialog to be closed
        await user.click(await screen.findByRole("button", {name: "Close"}));

        await waitFor(() => expect(api.RestoreSettings).toHaveBeenCalledWith(["vscode"]));
        await waitFor(() => expect(api.RestoreFolders).toHaveBeenCalledWith(["Proyectos"], "D:\\Nuevo"));
        expect(await screen.findByText("Settings: 4 files restored and 2 editor extensions.")).toBeInTheDocument();
        expect(screen.getByText("Folders: 7 written, 0 already there, 0 left as they were.")).toBeInTheDocument();
    });

    it("still restores settings if the installation is cancelled", async () => {
        api.RestoreSettings.mockResolvedValue({restored: 4, unchanged: 0, backedUp: 0, extensions: 0, skipped: []});
        api.Plan.mockResolvedValue({steps: [step("firefox", "Firefox")], alreadyInstalled: [], needsAdmin: false});
        const user = await open();
        await user.click(screen.getByRole("checkbox", {name: "Proyectos"}));
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        await user.click(await screen.findByRole("button", {name: "Cancel"}));
        await waitFor(() => expect(api.RestoreSettings).toHaveBeenCalled());
        expect(await screen.findByText("Apps: the installation was not run.")).toBeInTheDocument();
        expect(api.Apply).not.toHaveBeenCalled();
    });

    it("does not wait for an installation when there is nothing to install", async () => {
        api.RestoreSettings.mockResolvedValue({restored: 1, unchanged: 0, backedUp: 0, extensions: 0, skipped: []});
        api.Plan.mockResolvedValue({steps: [], alreadyInstalled: ["firefox"], needsAdmin: false});
        const user = await open();
        await user.click(screen.getByRole("checkbox", {name: "Proyectos"}));
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        await waitFor(() => expect(api.RestoreSettings).toHaveBeenCalled());
    });

    it("asks where to put the folders before touching anything", async () => {
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        expect(await screen.findByText("Choose where to put the folders.")).toBeInTheDocument();
        expect(api.RestoreSettings).not.toHaveBeenCalled();
        expect(api.Plan).not.toHaveBeenCalled();
    });

    it("reports a stage that fails without stopping the others", async () => {
        api.RestoreSettings.mockRejectedValue(new Error("access denied"));
        api.RestoreFolders.mockResolvedValue({written: 1, same: 0, kept: 0, skipped: [], dest: "D:\\N"});
        api.PickFolder.mockResolvedValue("D:\\N");
        api.Plan.mockResolvedValue({steps: [], alreadyInstalled: [], needsAdmin: false});
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Choose a folder…"}));
        await user.click(screen.getByRole("button", {name: "Rebuild now"}));
        expect(await screen.findByText("access denied")).toBeInTheDocument();
        await waitFor(() => expect(api.RestoreFolders).toHaveBeenCalled());
    });
});
