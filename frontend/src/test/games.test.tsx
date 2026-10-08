import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {Emulation} from "../api";
import App from "../App";
import {api, resetApi} from "./apiMock";
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
