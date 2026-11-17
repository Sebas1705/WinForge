import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {CleanCategory, ProjectJunk, VirtualDisk} from "../api";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

// The Tasks list repeats what a section reports, and sits above it: the section is the last match.
const lastText = async (x: string | RegExp) => (await screen.findAllByText(x)).at(-1) as HTMLElement;
const MB = 1024 ** 2;
const GB = 1024 ** 3;
const HOME = "C:\\Users\\me";

const junk = (project: string, kind: string, bytes: number, o: Partial<ProjectJunk> = {}): ProjectJunk => ({
    path: `${HOME}\\code\\${project}\\${kind === "rust-target" ? "target" : "node_modules"}`, project, projectPath: `${HOME}\\code\\${project}`, kind,
    sizeBytes: bytes, files: 100, lastTouched: "2026-01-10T10:00:00Z", ...o,
});
const found = (): ProjectJunk[] => [junk("webapp", "node_modules", 2 * GB), junk("engine", "rust-target", 800 * MB), junk("blog", "node_modules", 300 * MB)];
const disks: VirtualDisk[] = [
    {path: `${HOME}\\AppData\\Local\\Packages\\Ubuntu_x\\LocalState\\ext4.vhdx`, name: "Ubuntu22.04LTS", kind: "wsl", sizeBytes: 40 * GB, modified: "2026-05-01T10:00:00Z"},
    {path: `${HOME}\\AppData\\Local\\Docker\\wsl\\disk\\docker_data.vhdx`, name: "Docker Desktop", kind: "docker", sizeBytes: 12 * GB, modified: "2026-05-02T10:00:00Z"},
];

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({admin: true}));
    api.StorageProjects.mockImplementation(async () => found());
    api.StorageVirtualDisks.mockImplementation(async () => structuredClone(disks));
});

async function open(lang?: "es") {
    if (lang) localStorage.setItem("winforge.settings", JSON.stringify({language: lang}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(lang === "es" ? /apps del catálogo/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: lang === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: lang === "es" ? /Almacenamiento/ : /Storage/}));
    await screen.findByRole("heading", {name: lang === "es" ? "Almacenamiento" : "Storage", level: 1});
    return user;
}

const sec = (name = "Old project folders") => screen.getByRole("heading", {name, level: 2}).closest("section") as HTMLElement;

async function search(user: ReturnType<typeof userEvent.setup>) {
    await user.click(within(sec()).getByRole("button", {name: "Find old project folders"}));
    await within(sec()).findByRole("checkbox", {name: `Delete ${found()[0].path}`});
}

describe("storage: old project folders", () => {
    it("searches the home folder for 90 days by default, and lets people change the age and the folders", async () => {
        api.PickFolder.mockResolvedValue("D:\\Work");
        const user = await open();
        const s = sec();
        expect(await within(s).findByText(HOME)).toBeInTheDocument();
        await user.selectOptions(within(s).getByRole("combobox", {name: "Not touched for"}), "180");
        await user.click(within(s).getByRole("button", {name: "Add folder…"}));
        expect(await within(s).findByText("D:\\Work")).toBeInTheDocument();
        await user.click(within(s).getByRole("button", {name: "Find old project folders"}));
        await waitFor(() => expect(api.StorageProjects).toHaveBeenCalledWith([HOME, "D:\\Work"], 180));
    });

    it("cannot search without a folder", async () => {
        const user = await open();
        const s = sec();
        await user.click(await within(s).findByRole("button", {name: `Remove folder: ${HOME}`}));
        expect(within(s).getByText("Add at least one folder to search.")).toBeInTheDocument();
        expect(within(s).getByRole("button", {name: "Find old project folders"})).toBeDisabled();
    });

    it("lists what was found with its kind, starts with nothing ticked and keeps the delete button off", async () => {
        const user = await open();
        await search(user);
        const s = sec();
        expect(within(s).getAllByText("Node dependencies", {selector: ".chip"})[0]).toBeInTheDocument();
        expect(within(s).getByText("Rust build output")).toBeInTheDocument();
        for (const j of found()) expect((within(s).getByRole("checkbox", {name: `Delete ${j.path}`}) as HTMLInputElement).checked).toBe(false);
        expect(within(s).getByText("No folders selected")).toBeInTheDocument();
        expect(within(s).getByRole("button", {name: "Delete selected"})).toBeDisabled();
    });

    it("adds up what is ticked, and selects or clears everything", async () => {
        const user = await open();
        await search(user);
        const s = sec();
        await user.click(within(s).getByRole("checkbox", {name: `Delete ${found()[0].path}`}));
        expect(within(s).getByText(/1 folder, 2 GB can be freed/)).toBeInTheDocument();
        await user.click(within(s).getByRole("button", {name: "Select all"}));
        expect(within(s).getByText(/3 folders/)).toBeInTheDocument();
        await user.click(within(s).getByRole("button", {name: "Select none"}));
        expect(within(s).getByText("No folders selected")).toBeInTheDocument();
    });

    it("asks first, names every folder and says it cannot be undone, and does nothing on Cancel", async () => {
        const user = await open();
        await search(user);
        const s = sec();
        await user.click(within(s).getByRole("checkbox", {name: `Delete ${found()[1].path}`}));
        await user.click(within(s).getByRole("button", {name: "Delete selected"}));
        const dialog = await screen.findByRole("alertdialog");
        expect(within(dialog).getByText(/cannot be undone/)).toBeInTheDocument();
        expect(within(dialog).getByText(found()[1].path)).toBeInTheDocument();
        await user.click(within(dialog).getByRole("button", {name: "Cancel"}));
        expect(api.StorageProjectsClean).not.toHaveBeenCalled();
    });

    it("sends only the ticked folders, shows what was freed and then searches again", async () => {
        api.StorageProjectsClean.mockResolvedValue({freedBytes: 800 * MB, removed: 1, files: 100, failed: [], reasons: []});
        const user = await open();
        await search(user);
        const s = sec();
        await user.click(within(s).getByRole("checkbox", {name: `Delete ${found()[1].path}`}));
        await user.click(within(s).getByRole("button", {name: "Delete selected"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Delete"}));
        await waitFor(() => expect(api.StorageProjectsClean).toHaveBeenCalledWith([found()[1].path]));
        expect(await within(sec()).findByText(/Deleted 1, freed 800 MB\./)).toBeInTheDocument();
        await waitFor(() => expect(api.StorageProjects).toHaveBeenCalledTimes(2));
        expect(api.StorageVolumes).toHaveBeenCalledTimes(2); // the drives are measured again
    });

    it("says which folders could not be deleted and why", async () => {
        const bad = found()[0].path;
        api.StorageProjectsClean.mockResolvedValue({freedBytes: 0, removed: 0, files: 0, failed: [bad], reasons: ["the project was touched 3h ago"]});
        const user = await open();
        await search(user);
        const s = sec();
        await user.click(within(s).getByRole("checkbox", {name: `Delete ${bad}`}));
        await user.click(within(s).getByRole("button", {name: "Delete selected"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Delete"}));
        expect(await within(sec()).findByText(`${bad}: the project was touched 3h ago`)).toBeInTheDocument();
        expect(within(sec()).getByText("1 could not be deleted:")).toBeInTheDocument();
    });

    it("shows nothing found as a message, and a failed search as a message", async () => {
        api.StorageProjects.mockResolvedValueOnce([]);
        const user = await open();
        await user.click(within(sec()).getByRole("button", {name: "Find old project folders"}));
        expect(await within(sec()).findByText("No old project folders were found.")).toBeInTheDocument();
        api.StorageProjects.mockRejectedValueOnce(new Error("boom"));
        await user.click(within(sec()).getByRole("button", {name: "Scan again"}));
        expect(await lastText(/boom/)).toBeInTheDocument();
    });

    it("shows a live search with Cancel, and ignores other tasks' progress", async () => {
        let release: (v: ProjectJunk[]) => void = () => undefined;
        api.StorageProjects.mockImplementationOnce(() => new Promise<ProjectJunk[]>((r) => { release = r; }));
        const user = await open();
        await user.click(within(sec()).getByRole("button", {name: "Find old project folders"}));
        expect(await within(sec()).findByText("Looking for old project folders…")).toBeInTheDocument();
        emit.storageProgress({task: "projects", files: 1200, bytes: 5 * MB, current: `${HOME}\\code\\webapp`});
        expect(await within(sec()).findByText(/1,200 files|1200 files/)).toBeInTheDocument();
        await user.click(within(sec()).getByRole("button", {name: "Cancel: Looking for old project folders…"}));
        expect(api.StorageCancel).toHaveBeenCalled();
        release([]);
    });

    it("opens a found folder in Explorer", async () => {
        const user = await open();
        await search(user);
        await user.click(within(sec()).getByRole("button", {name: "Show in folder: webapp (Node dependencies)"}));
        expect(api.StorageReveal).toHaveBeenCalledWith(found()[0].path);
    });

    it("is in Spanish, with its kinds and the warning that it cannot be undone", async () => {
        const user = await open("es");
        const s = sec("Carpetas de proyectos antiguos");
        await user.click(within(s).getByRole("button", {name: "Buscar carpetas de proyectos antiguos"}));
        const first = await within(sec("Carpetas de proyectos antiguos")).findByRole("checkbox", {name: `Borrar ${found()[0].path}`});
        await user.click(first);
        expect(within(s).getAllByText("Dependencias de Node", {selector: ".chip"})[0]).toBeInTheDocument();
        await user.click(within(s).getByRole("button", {name: "Borrar las marcadas"}));
        expect(within(await screen.findByRole("alertdialog")).getByText(/No se puede deshacer/)).toBeInTheDocument();
    });
});

describe("storage: virtual disks", () => {
    it("lists WSL and Docker disks with their size, where they are, and only offers to show them", async () => {
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Look for virtual disks"}));
        expect(await screen.findByText("Ubuntu22.04LTS · 40 GB")).toBeInTheDocument();
        expect(screen.getByText("Docker Desktop · 12 GB")).toBeInTheDocument();
        expect(screen.getByText(disks[0].path)).toBeInTheDocument();
        expect(screen.getByText(/run wsl --shutdown and compact it/)).toBeInTheDocument();
        expect(screen.queryByRole("checkbox", {name: /Ubuntu/})).toBeNull();
        await user.click(screen.getByRole("button", {name: "Show in folder: Ubuntu22.04LTS"}));
        expect(api.StorageReveal).toHaveBeenCalledWith(disks[0].path);
    });

    it("says when there are none, and shows an error as a message", async () => {
        api.StorageVirtualDisks.mockResolvedValueOnce([]);
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Look for virtual disks"}));
        expect(await screen.findByText("No WSL or Docker virtual disks were found.")).toBeInTheDocument();
        api.StorageVirtualDisks.mockRejectedValueOnce(new Error("denied"));
        await user.click(screen.getByRole("button", {name: "Look for virtual disks"}));
        expect(await screen.findByText(/denied/)).toBeInTheDocument();
    });

    it("is in Spanish", async () => {
        const user = await open("es");
        await user.click(screen.getByRole("button", {name: "Buscar discos virtuales"}));
        expect(await screen.findByText("Docker Desktop · 12 GB")).toBeInTheDocument();
        expect(screen.getAllByText(/Modificado/, {selector: "small"})).toHaveLength(2);
    });
});

describe("storage: developer cache categories", () => {
    const cat = (id: string, bytes: number, o: Partial<CleanCategory> = {}): CleanCategory => ({id, risk: "review", needsAdmin: false, available: true, bytes, files: 12, skipped: 0, ...o});

    it("names the new categories and does not invent a file count for Docker", async () => {
        api.StorageScan.mockResolvedValue([cat("dev-yarn", 100 * MB, {risk: "safe"}), cat("dev-cargo", 2 * GB), cat("docker-build-cache", 5 * GB, {files: 12}), cat("docker-dangling", GB, {files: 3})]);
        const user = await open();
        await user.click(screen.getByRole("button", {name: /Scan for junk/}));
        expect(await screen.findByRole("checkbox", {name: "Select for cleaning: Docker build cache"})).toBeInTheDocument();
        for (const n of ["Yarn cache", "Cargo registry cache", "Docker unused images"]) expect(screen.getByText(n)).toBeInTheDocument();
        const dockerRow = screen.getByRole("checkbox", {name: "Select for cleaning: Docker build cache"}).closest("label") as HTMLElement;
        expect(within(dockerRow).queryByText(/files?$/)).toBeNull();
        expect(within(dockerRow).getByText(/Containers and volumes are not touched/)).toBeInTheDocument();
        const yarnRow = screen.getByRole("checkbox", {name: "Select for cleaning: Yarn cache"}).closest("label") as HTMLElement;
        expect(within(yarnRow).getByText("12 files")).toBeInTheDocument();
    });
});
