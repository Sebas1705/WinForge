import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {Emulation} from "../api";
import App from "../App";
import {api, emit, resetApi} from "./apiMock";
import {app, makeState} from "./fixtures";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const apps = [
    app({id: "dolphin", name: "Dolphin", category: "gaming/emulators", installed: true}),
    app({id: "mgba", name: "mGBA", category: "gaming/emulators", installed: true}),
    app({id: "pcsx2", name: "PCSX2", category: "gaming/emulators", installed: false}),
    app({id: "git", name: "Git"}),
];

const emulation: Emulation = {
    systems: [
        {id: "gamecube", en: "GameCube", es: "GameCube"}, {id: "wii", en: "Wii", es: "Wii"},
        {id: "gba", en: "Game Boy Advance", es: "Game Boy Advance"}, {id: "ps2", en: "PlayStation 2", es: "PlayStation 2"},
    ],
    emulators: {dolphin: ["gamecube", "wii"], mgba: ["gba"], pcsx2: ["ps2"]},
    sources: [
        {id: "wiibrew", system: "wii", kind: "community", name: "WiiBrew", en: "The Wii homebrew wiki.", es: "La wiki de homebrew de Wii.", url: "https://wiibrew.org/"},
        {id: "itch-gba", system: "gba", kind: "homebrew", name: "itch.io: GBA games", en: "New GBA games.", es: "Juegos nuevos de GBA.", url: "https://itch.io/games/tag-gba"},
        {id: "mother3", system: "gba", kind: "translations", name: "Mother 3 translation", en: "A fan translation, as a patch.", es: "Una traducción de fans, en parche.", url: "https://mother3.fobby.net/", base: true},
        {id: "ps2-thing", system: "ps2", kind: "homebrew", name: "PS2 thing", en: "Not shown: PCSX2 is not installed.", es: "No se muestra.", url: "https://example.org/"},
        {id: "romhacking", system: "any", kind: "hacks", name: "Romhacking.net", en: "Patches for many games.", es: "Parches para muchos juegos.", url: "https://www.romhacking.net/hacks/", base: true},
    ],
};

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi(makeState({apps}));
    api.Emulation.mockResolvedValue(emulation);
});

async function open(state = makeState({apps})) {
    api.GetState.mockImplementation(async () => structuredClone(state));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Games for emulators/}));
    await screen.findByRole("heading", {name: "Games for your emulators"});
    return user;
}

describe("games for your emulators", () => {
    it("finds the installed emulators, and only the systems they play", async () => {
        await open();
        const mine = screen.getByRole("heading", {name: "Emulators found on this PC"}).closest("section") as HTMLElement;
        expect(within(mine).getByRole("button", {name: "Dolphin"})).toBeInTheDocument();
        expect(within(mine).getByRole("button", {name: "mGBA"})).toBeInTheDocument();
        expect(within(mine).queryByRole("button", {name: "PCSX2"})).not.toBeInTheDocument(); // not installed
        expect(screen.getByRole("region", {name: "Game Boy Advance"})).toBeInTheDocument();
        expect(screen.getByRole("region", {name: "Wii"})).toBeInTheDocument();
        expect(screen.queryByRole("region", {name: "PlayStation 2"})).not.toBeInTheDocument();
        expect(screen.queryByText("PS2 thing")).not.toBeInTheDocument();
        expect(screen.getAllByText("Plays: Dolphin").length).toBe(2); // GameCube and Wii
    });

    it("says which links need your own copy of the game, and opens a link in the browser", async () => {
        const user = await open();
        const gba = screen.getByRole("region", {name: "Game Boy Advance"});
        const mother = within(gba).getByText("Mother 3 translation").closest("li") as HTMLElement;
        expect(within(mother).getByText("needs your own copy of the game")).toBeInTheDocument();
        const homebrew = within(gba).getByText("itch.io: GBA games").closest("li") as HTMLElement;
        expect(within(homebrew).queryByText("needs your own copy of the game")).not.toBeInTheDocument();
        await user.click(within(homebrew).getByRole("button", {name: "Open page: itch.io: GBA games"}));
        expect(api.OpenURL).toHaveBeenCalledWith("https://itch.io/games/tag-gba");
    });

    it("always shows the general sources, and the legal note", async () => {
        await open();
        const any = screen.getByRole("heading", {name: "For any system"}).closest("section") as HTMLElement;
        expect(within(any).getByText("Romhacking.net")).toBeInTheDocument();
        expect(screen.getByRole("note")).toHaveTextContent("does not download or host games");
    });

    it("offers to browse emulators when none is installed", async () => {
        const user = await open(makeState({apps: apps.map((a) => ({...a, installed: false}))}));
        expect(screen.getByText("No emulators from the catalog are installed yet.")).toBeInTheDocument();
        expect(screen.queryByRole("region", {name: "Wii"})).not.toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Browse emulators"}));
        expect(await screen.findByRole("tab", {name: /Gaming/, selected: true})).toBeInTheDocument();
    });
});

describe("the patch tool", () => {
    it("reports where the patched game went and whether the version was confirmed", async () => {
        api.PatchROM.mockResolvedValue({path: "C:\\roms\\Game (patched).gba", format: "BPS", checked: true, headerless: false});
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Patch a game…"}));
        expect(await screen.findByText(/Patched game saved as C:\\roms\\Game \(patched\)\.gba\. The patch confirmed this is the right version/)).toBeInTheDocument();
    });

    it("warns that an IPS patch cannot confirm the version", async () => {
        api.PatchROM.mockResolvedValue({path: "C:\\g (patched).sfc", format: "IPS", checked: false, headerless: false});
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Patch a game…"}));
        expect(await screen.findByText(/cannot confirm the game version/)).toBeInTheDocument();
    });

    it("mentions a removed copier header", async () => {
        api.PatchROM.mockResolvedValue({path: "C:\\g (patched).sfc", format: "UPS", checked: true, headerless: true});
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Patch a game…"}));
        expect(await screen.findByText(/512-byte header was removed/)).toBeInTheDocument();
    });

    it("shows the reason when the patch does not fit the game, and says nothing when cancelled", async () => {
        api.PatchROM.mockRejectedValueOnce(new Error("this patch is for a different version of the game"));
        const user = await open();
        await user.click(screen.getByRole("button", {name: "Patch a game…"}));
        expect(await screen.findByText("this patch is for a different version of the game")).toBeInTheDocument();
        api.PatchROM.mockResolvedValueOnce({path: "", format: "", checked: false, headerless: false});
        await user.click(screen.getByRole("button", {name: "Patch a game…"}));
        await waitFor(() => expect(api.PatchROM).toHaveBeenCalledTimes(2));
        expect(screen.queryByText(/Patched game saved/)).not.toBeInTheDocument();
    });
});

describe("installing games", () => {
    const gba = {
        id: "libbet", name: "Libbet and the Magic Fish", system: "gba", en: "A free puzzle game.", es: "Un juego de puzles.", license: "zlib",
        homepage: "https://github.com/pinobatch/libbet", url: "https://github.com/x/libbet.gb", sha256: "a".repeat(64), size: 32768, kind: "file" as const, file: "libbet.gb", entry: "libbet.gb",
    };
    const scumm = {...gba, id: "bass", name: "Beneath a Steel Sky", system: "scumm", size: 69377781, kind: "zip" as const, entry: undefined, file: undefined};
    const withRun: Emulation = {
        ...emulation,
        systems: [...emulation.systems, {id: "scumm", en: "Point-and-click adventures", es: "Aventuras gráficas"}],
        emulators: {...emulation.emulators, scummvm: ["scumm"]},
        run: {mgba: {exes: ["mGBA.exe"], hints: ["mgba"], args: ["{file}"]}, scummvm: {exes: ["scummvm.exe"], hints: ["scummvm"], register: ["--add"]}},
    };
    const allApps = [...apps, app({id: "scummvm", name: "ScummVM", category: "gaming/emulators", installed: true})];

    function view(o: Partial<{installed: Record<string, unknown>; launchers: Record<string, string>}> = {}) {
        return {root: "C:\WinForge Games", games: [gba, scumm], installed: {}, launchers: {mgba: "C:\Tools\mGBA\mGBA.exe"}, ...o};
    }
    async function openWith(v = view()) {
        api.Emulation.mockResolvedValue(withRun);
        api.GamesState.mockResolvedValue(v as never);
        return open(makeState({apps: allApps}));
    }

    it("offers each game under the system it belongs to, with its license and size", async () => {
        await openWith();
        const g = screen.getByRole("region", {name: "Game Boy Advance"});
        const row = within(g).getByText("Libbet and the Magic Fish").closest("li") as HTMLElement;
        expect(within(row).getByText(/License: zlib · 32 KB/)).toBeInTheDocument();
        expect(within(row).getByRole("button", {name: "Install: Libbet and the Magic Fish"})).toBeInTheDocument();
        expect(screen.getByRole("region", {name: "Point-and-click adventures"})).toBeInTheDocument();
        expect(screen.getByText("C:\WinForge Games")).toBeInTheDocument();
    });

    it("downloads with progress, then offers to play and remove", async () => {
        let finish!: (r: unknown) => void;
        api.InstallGame.mockReturnValue(new Promise((res) => { finish = res; }));
        const user = await openWith();
        await user.click(screen.getByRole("button", {name: "Install: Libbet and the Magic Fish"}));
        expect(api.InstallGame).toHaveBeenCalledWith("libbet");
        await emit.gameProgress("libbet", 16384, 32768);
        expect(await screen.findByText("Installing… 50%")).toBeInTheDocument();
        expect(screen.getByRole("progressbar", {name: "Libbet and the Magic Fish"})).toHaveValue(50);
        // The page refreshes after the install, which now reports it as installed.
        api.GamesState.mockResolvedValue(view({installed: {libbet: {id: "libbet"}}}) as never);
        finish({receipt: {id: "libbet"}, registered: []});
        expect(await screen.findByRole("button", {name: "Play with mGBA: Libbet and the Magic Fish"})).toBeInTheDocument();
        expect(screen.getByText((c) => c.startsWith("Libbet and the Magic Fish installed in ") && c.endsWith("WinForge Games\\gba\\libbet."))).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: "Install: Libbet and the Magic Fish"})).not.toBeInTheDocument();
    });

    it("starts the game with the emulator the person picks", async () => {
        const user = await openWith(view({installed: {libbet: {id: "libbet"}}}));
        await user.click(await screen.findByRole("button", {name: "Play with mGBA: Libbet and the Magic Fish"}));
        expect(api.PlayGame).toHaveBeenCalledWith("libbet", "mgba");
        await user.click(screen.getByRole("button", {name: "Open folder: Libbet and the Magic Fish"}));
        expect(api.OpenGameFolder).toHaveBeenCalledWith("libbet");
    });

    it("says where the game is when the emulator cannot be started by WinForge", async () => {
        await openWith(view({installed: {libbet: {id: "libbet"}}, launchers: {}}));
        const where = await screen.findByText((c) => c.startsWith("Open it from mGBA: the game is in ") && c.endsWith("WinForge Games\\gba\\libbet."));
        expect(where).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Play with/})).not.toBeInTheDocument();
    });

    it("reports that ScummVM was told about a game, or that it could not be", async () => {
        api.InstallGame.mockResolvedValueOnce({receipt: {id: "bass"}, registered: ["scummvm"]});
        const user = await openWith();
        await user.click(screen.getByRole("button", {name: "Install: Beneath a Steel Sky"}));
        expect(await screen.findByText(/It was added to ScummVM\./)).toBeInTheDocument();
        api.InstallGame.mockResolvedValueOnce({receipt: {id: "bass"}, registered: [], registerError: "scummvm: bad option"});
        await user.click(screen.getByRole("button", {name: "Install: Beneath a Steel Sky"}));
        expect(await screen.findByText(/ScummVM could not be told about it \(bad option\)/)).toBeInTheDocument();
    });

    it("shows why an install failed and lets the person try again", async () => {
        api.InstallGame.mockRejectedValueOnce(new Error("Libbet does not match its published checksum, so it was discarded"));
        const user = await openWith();
        await user.click(screen.getByRole("button", {name: "Install: Libbet and the Magic Fish"}));
        expect(await screen.findByText(/does not match its published checksum/)).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Install: Libbet and the Magic Fish"})).toBeEnabled();
        expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    });

    it("asks before removing, and only then removes", async () => {
        const user = await openWith(view({installed: {libbet: {id: "libbet"}}}));
        await user.click(await screen.findByRole("button", {name: "Remove: Libbet and the Magic Fish"}));
        const sure = await screen.findByRole("alertdialog", {name: "Remove Libbet and the Magic Fish?"});
        expect(api.RemoveGame).not.toHaveBeenCalled();
        await user.click(within(sure).getByRole("button", {name: "Cancel"}));
        expect(api.RemoveGame).not.toHaveBeenCalled();
        await user.click(screen.getByRole("button", {name: "Remove: Libbet and the Magic Fish"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Remove"}));
        await waitFor(() => expect(api.RemoveGame).toHaveBeenCalledWith("libbet"));
    });

    it("lets the person change the games folder", async () => {
        api.SetGamesRoot.mockResolvedValue("D:\Juegos");
        const user = await openWith();
        api.GamesState.mockResolvedValue({...view(), root: "D:\Juegos"} as never);
        await user.click(screen.getByRole("button", {name: "Change…"}));
        expect(await screen.findByText("D:\Juegos")).toBeInTheDocument();
    });

    it("tells how many games wait for an emulator that is not installed", async () => {
        api.Emulation.mockResolvedValue(withRun);
        api.GamesState.mockResolvedValue(view() as never);
        await open(makeState({apps})); // ScummVM is not installed here
        expect(await screen.findByText("1 free games are waiting for an emulator (Point-and-click adventures).")).toBeInTheDocument();
    });
});
