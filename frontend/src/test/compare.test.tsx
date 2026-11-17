import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {CompareResult} from "../api";
import App from "../App";
import {api, resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

const FILE = "C:\\Users\\me\\Downloads\\laptop.winforge.json";

const side = (o: Partial<CompareResult["a"]> = {}): CompareResult["a"] => ({
    kind: "pc", name: "", file: "", apps: 0, versioned: true, unknownRecipes: [], unknownProfiles: [], profile: null, ...o,
});
const laptop = side({
    kind: "file", name: "Laptop", file: "laptop.winforge.json", apps: 3, versioned: false,
    profile: {id: "imported", name: "Laptop", kind: "custom", apps: [{id: "firefox"}, {id: "git"}, {id: "vlc"}]},
});
const result = (o: Partial<CompareResult> = {}): CompareResult => ({
    a: side(), b: laptop,
    onlyA: [{id: "gimp", name: "GIMP"}],
    onlyB: [{id: "firefox", name: "Firefox"}, {id: "git", name: "Git"}],
    both: [{id: "vlc", name: "VLC"}],
    differ: [], unknown: [{id: "ghost-app", in: "b"}], ...o,
});

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
    api.PickProfileFile.mockResolvedValue(FILE);
    api.CompareFile.mockResolvedValue(result());
});

async function open(language: "en" | "es" = "en") {
    if (language === "es") localStorage.setItem("winforge.settings", JSON.stringify({language: "es"}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(language === "es" ? /apps del catálogo encontradas/ : /catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: language === "es" ? "Menú principal" : "Main menu"})).getByRole("button", {name: /Perfiles|Profiles/}));
    await user.click(await screen.findByRole("button", {name: language === "es" ? "Comparar…" : "Compare…"}));
    return user;
}

const chooseFile = async (user: ReturnType<typeof userEvent.setup>, name = "Choose a file…") => {
    await user.click(screen.getByRole("button", {name}));
};

describe("comparing this PC with a profile file", () => {
    it("is entered from the Profiles page and goes back to it", async () => {
        const user = await open();
        expect(screen.getByRole("heading", {level: 1, name: "Compare setups"})).toBeInTheDocument();
        expect(screen.getByText(/Comparing only reads: nothing changes on this PC/)).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Back to profiles"}));
        expect(screen.getByRole("button", {name: "Compare…"})).toBeInTheDocument();
    });

    it("says how to compare with a saved profile instead of duplicating it", async () => {
        await open();
        expect(screen.getByText(/open that profile: what changed since it was applied is listed there/)).toBeInTheDocument();
    });

    it("chooses a file and lists the groups with counts, a summary and no change to the PC", async () => {
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByRole("heading", {name: "Only on this PC (1)"})).toBeInTheDocument();
        expect(api.PickProfileFile).toHaveBeenCalledOnce();
        expect(api.CompareFile).toHaveBeenCalledWith(FILE);
        expect(screen.getByRole("heading", {name: "Only in the file (2)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "In both (1)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Unknown to this catalog (1)"})).toBeInTheDocument();
        expect(screen.queryByRole("heading", {name: /Versions differ/})).not.toBeInTheDocument(); // the file has no versions
        expect(screen.getByRole("status")).toHaveTextContent("In both: 1. Only with this PC: 1. Only with the file “Laptop”: 2. Not in this catalog: 1.");
        expect(screen.getByText("laptop.winforge.json")).toBeInTheDocument();
        const only = within(screen.getByRole("list", {name: "Only in the file"}));
        expect(only.getAllByRole("listitem").map((x) => x.querySelector(".cmp-name")?.textContent)).toEqual(["Firefox", "Git"]);
        expect(within(screen.getByRole("list", {name: "Unknown to this catalog"})).getByText("ghost-app")).toBeInTheDocument();
        expect(screen.getByText(/Versions are compared only when both sides list them/)).toBeInTheDocument();
        expect(api.SaveProfile).not.toHaveBeenCalled();
        expect(api.Apply).not.toHaveBeenCalled();
    });

    it("keeps 'In both' collapsed until it is asked for, and tells assistive technology", async () => {
        const user = await open();
        await chooseFile(user);
        await screen.findByRole("heading", {name: "In both (1)"});
        expect(screen.queryByRole("list", {name: "In both"})).not.toBeInTheDocument();
        const toggle = screen.getByRole("button", {name: "Show the list"});
        expect(toggle).toHaveAttribute("aria-expanded", "false");
        await user.click(toggle);
        expect(screen.getByRole("button", {name: "Hide the list"})).toHaveAttribute("aria-expanded", "true");
        expect(within(screen.getByRole("list", {name: "In both"})).getByText("VLC")).toBeInTheDocument();
    });

    it("shows long lists in part, with show more and show fewer", async () => {
        const many = Array.from({length: 30}, (_, i) => ({id: `app${i}`, name: `App ${String(i).padStart(2, "0")}`}));
        api.CompareFile.mockResolvedValue(result({onlyB: many}));
        const user = await open();
        await chooseFile(user);
        const list = await screen.findByRole("list", {name: "Only in the file"});
        expect(within(list).getAllByRole("listitem")).toHaveLength(12);
        expect(screen.getByRole("heading", {name: "Only in the file (30)"})).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Show 18 more"}));
        expect(within(screen.getByRole("list", {name: "Only in the file"})).getAllByRole("listitem")).toHaveLength(30);
        await user.click(screen.getByRole("button", {name: "Show fewer"}));
        expect(within(screen.getByRole("list", {name: "Only in the file"})).getAllByRole("listitem")).toHaveLength(12);
    });

    it("does nothing when the file chooser is cancelled", async () => {
        api.PickProfileFile.mockResolvedValue("");
        const user = await open();
        await chooseFile(user);
        expect(api.CompareFile).not.toHaveBeenCalled();
        expect(screen.queryByRole("status")).not.toBeInTheDocument();
    });

    it("lists the versions that differ only when both sides have versions", async () => {
        api.CompareFile.mockResolvedValue(result({
            b: {...laptop, versioned: true},
            differ: [{id: "git", name: "Git", a: "2.51", b: "2.40"}], onlyB: [], unknown: [],
        }));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByRole("heading", {name: "Versions differ (1)"})).toBeInTheDocument();
        expect(screen.getByText("2.51 on this PC, 2.40 in the file")).toBeInTheDocument();
        expect(screen.getByRole("status")).toHaveTextContent("Versions differ for 1.");
        expect(screen.queryByText(/Versions are compared only when both sides list them/)).not.toBeInTheDocument();
    });

    it("says so when the two are the same", async () => {
        api.CompareFile.mockResolvedValue(result({onlyA: [], onlyB: [], unknown: [], both: [{id: "vlc", name: "VLC"}, {id: "git", name: "Git"}]}));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByRole("status")).toHaveTextContent("Both sides have the same apps (2).");
        expect(screen.getAllByText("None.")).toHaveLength(2);
    });

    it("names what was ignored in the file, never run", async () => {
        api.CompareFile.mockResolvedValue(result({b: {...laptop, unknownRecipes: ["evil-step"], unknownProfiles: ["not-mine"]}}));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByText(/never run: evil-step/)).toBeInTheDocument();
        expect(screen.getByText(/does not have were ignored: not-mine/)).toBeInTheDocument();
    });
});

describe("what the result lets you do", () => {
    it("installs the apps only in the file through the usual plan", async () => {
        const user = await open();
        await chooseFile(user);
        const button = await screen.findByRole("button", {name: /Install the missing apps \(2\)/});
        expect(button).toHaveAttribute("title", expect.stringContaining("review it before anything is installed"));
        await user.click(button);
        await waitFor(() => expect(api.Plan).toHaveBeenCalledOnce());
        expect(api.Plan.mock.calls[0][0].apps).toEqual([{id: "firefox"}, {id: "git"}]);
        expect(api.Apply).not.toHaveBeenCalled(); // the plan is shown first
    });

    it("does not offer to install when there is nothing missing", async () => {
        api.CompareFile.mockResolvedValue(result({onlyB: []}));
        const user = await open();
        await chooseFile(user);
        await screen.findByRole("status");
        expect(screen.queryByRole("button", {name: /Install the missing apps/})).not.toBeInTheDocument();
    });

    it("adds the apps only on this PC to one of your own profiles, keeping what it had", async () => {
        const user = await open();
        await chooseFile(user);
        const select = await screen.findByRole("combobox", {name: "Profile"});
        expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Mine"]); // built-ins are not offered
        await user.click(screen.getByRole("button", {name: "Add to the profile (1)"}));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalledOnce());
        expect(api.SaveProfile.mock.calls[0][0]).toMatchObject({id: "mine", name: "Mine", apps: [{id: "gimp"}]});
        expect(await screen.findByText("Added to “Mine”: 1.")).toBeInTheDocument();
    });

    it("adds new apps next to the ones the profile already lists", async () => {
        api.CompareFile.mockResolvedValue(result({onlyA: [{id: "vlc", name: "VLC"}, {id: "gimp", name: "GIMP"}]}));
        const user = await open();
        await chooseFile(user);
        await user.click(await screen.findByRole("button", {name: "Add to the profile (2)"}));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalledOnce());
        expect(api.SaveProfile.mock.calls[0][0].apps).toEqual([{id: "gimp"}, {id: "vlc"}]);
    });

    it("explains that there is no profile of your own to add to", async () => {
        const state = (await api.GetState()) as { profiles: { builtin: boolean }[] };
        state.profiles = state.profiles.filter((x) => x.builtin);
        api.GetState.mockImplementation(async () => structuredClone(state));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByText(/You have no profiles of your own yet/)).toBeInTheDocument();
        expect(screen.queryByRole("combobox", {name: "Profile"})).not.toBeInTheDocument();
    });

    it("saves the file as a profile with a name and id that collide with nothing", async () => {
        api.CompareFile.mockResolvedValue(result({b: {...laptop, name: "Mine"}})); // "Mine" already exists
        const user = await open();
        await chooseFile(user);
        await user.click(await screen.findByRole("button", {name: "Save the file as a profile"}));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalledOnce());
        expect(api.SaveProfile.mock.calls[0][0]).toMatchObject({id: "mine-2", name: "Mine (2)", apps: [{id: "firefox"}, {id: "git"}, {id: "vlc"}]});
        expect(await screen.findByText("Saved as “Mine (2)”.")).toBeInTheDocument();
    });

    it("reports a failure to save instead of closing silently", async () => {
        api.SaveProfile.mockRejectedValue(new Error("disk full"));
        const user = await open();
        await chooseFile(user);
        await user.click(await screen.findByRole("button", {name: "Save the file as a profile"}));
        expect(await screen.findByText("The comparison failed: disk full")).toBeInTheDocument();
    });
});

describe("comparing with a share code", () => {
    it("sends the pasted code and shows the result", async () => {
        api.CompareCode.mockResolvedValue(result({b: {...laptop, kind: "code", name: "", file: ""}}));
        const user = await open();
        await user.click(screen.getByRole("radio", {name: "This PC and a share code"}));
        const compare = screen.getByRole("button", {name: "Compare"});
        expect(compare).toBeDisabled();
        await user.type(screen.getByRole("textbox", {name: "Share code"}), "WF1.abc");
        await user.click(compare);
        await waitFor(() => expect(api.CompareCode).toHaveBeenCalledWith("WF1.abc"));
        expect(await screen.findByRole("heading", {name: "Only in the code (2)"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Save the code as a profile"})).toBeInTheDocument();
        expect(api.CompareFile).not.toHaveBeenCalled();
    });

    it("words a bad code in the language of the interface", async () => {
        api.CompareCode.mockRejectedValue(new Error("compare:cut-off"));
        const user = await open();
        await user.click(screen.getByRole("radio", {name: "This PC and a share code"}));
        await user.type(screen.getByRole("textbox", {name: "Share code"}), "WF1.abc");
        await user.click(screen.getByRole("button", {name: "Compare"}));
        expect(await screen.findByRole("alert")).toHaveTextContent("That share code looks cut off or altered.");
    });
});

describe("comparing two profile files", () => {
    const pa = "C:\\a\\desktop.json";
    const pb = "C:\\b\\laptop.json";
    const files = () => result({
        a: side({kind: "file", name: "Desktop", file: "desktop.json", apps: 2, profile: {id: "imported", name: "Desktop", kind: "custom", apps: [{id: "vlc"}]}}),
        b: side({kind: "file", name: "Laptop", file: "laptop.json", apps: 2, profile: {id: "imported", name: "Laptop", kind: "custom", apps: [{id: "git"}]}}),
        onlyA: [{id: "vlc", name: "VLC"}], onlyB: [{id: "git", name: "Git"}], both: [], unknown: [{id: "ghost", in: "both"}],
    });

    it("needs both files, then compares them and labels the groups A and B", async () => {
        api.PickProfileFile.mockResolvedValueOnce(pa).mockResolvedValueOnce(pb);
        api.CompareFiles.mockResolvedValue(files());
        const user = await open();
        await user.click(screen.getByRole("radio", {name: "Two profile files"}));
        const compare = screen.getByRole("button", {name: "Compare"});
        expect(compare).toBeDisabled();
        await user.click(screen.getByRole("button", {name: "Choose file A…"}));
        expect(await screen.findByText("File A: desktop.json")).toBeInTheDocument();
        expect(compare).toBeDisabled();
        await user.click(screen.getByRole("button", {name: "Choose file B…"}));
        expect(await screen.findByText("File B: laptop.json")).toBeInTheDocument();
        await user.click(compare);
        await waitFor(() => expect(api.CompareFiles).toHaveBeenCalledWith(pa, pb));
        expect(await screen.findByRole("heading", {name: "Only in file A (1)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Only in file B (1)"})).toBeInTheDocument();
        expect(screen.getByText("in both files")).toBeInTheDocument();
        expect(screen.getByText("Comparing file A “Desktop” with file B “Laptop”.")).toBeInTheDocument();
        expect(screen.queryByRole("button", {name: /Install the missing apps/})).not.toBeInTheDocument(); // neither side is this PC
        expect(screen.queryByRole("button", {name: /to the profile/})).not.toBeInTheDocument();
        expect(api.CompareFile).not.toHaveBeenCalled();
    });

    it("saves either file as a profile", async () => {
        api.PickProfileFile.mockResolvedValueOnce(pa).mockResolvedValueOnce(pb);
        api.CompareFiles.mockResolvedValue(files());
        const user = await open();
        await user.click(screen.getByRole("radio", {name: "Two profile files"}));
        await user.click(screen.getByRole("button", {name: "Choose file A…"}));
        await user.click(screen.getByRole("button", {name: "Choose file B…"}));
        await user.click(screen.getByRole("button", {name: "Compare"}));
        await user.click(await screen.findByRole("button", {name: "Save file B as a profile"}));
        await waitFor(() => expect(api.SaveProfile).toHaveBeenCalledOnce());
        expect(api.SaveProfile.mock.calls[0][0]).toMatchObject({id: "laptop", name: "Laptop", apps: [{id: "git"}]});
        expect(screen.getByRole("button", {name: "Save file A as a profile"})).toBeInTheDocument();
    });
});

describe("errors", () => {
    it.each([
        ["compare:too-big", "That is too big to be a profile."],
        ["compare:not-json", "That is not a readable profile: it may be damaged or cut off."],
        ["compare:not-profile", "That is not a WinForge profile."],
        ["compare:newer-format", "That profile was made by a newer WinForge. Update WinForge to read it."],
        ["compare:no-scan", "This PC has not been scanned yet. Scan it first, then compare."],
        ["compare:unreadable", "That file could not be read."],
        ["compare:not-picked", "Choose the file again, please."],
        ["compare:something-new", "The comparison failed: compare:something-new"],
        ["boom", "The comparison failed: boom"],
    ])("shows %s in plain words", async (code, text) => {
        api.CompareFile.mockRejectedValue(new Error(code));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByRole("alert")).toHaveTextContent(text);
        expect(screen.queryByRole("status")).not.toBeInTheDocument();
    });

    it("shows a failure of the file chooser itself", async () => {
        api.PickProfileFile.mockRejectedValue(new Error("dialog failed"));
        const user = await open();
        await chooseFile(user);
        expect(await screen.findByRole("alert")).toHaveTextContent("The comparison failed: dialog failed");
    });

    it("drops an old result when a new comparison fails", async () => {
        api.CompareFile.mockResolvedValueOnce(result()).mockRejectedValueOnce(new Error("compare:not-profile"));
        const user = await open();
        await chooseFile(user);
        await screen.findByRole("status");
        await chooseFile(user);
        expect(await screen.findByRole("alert")).toBeInTheDocument();
        expect(screen.queryByRole("status")).not.toBeInTheDocument();
    });

    it("clears the result when another kind of comparison is picked", async () => {
        const user = await open();
        await chooseFile(user);
        await screen.findByRole("status");
        await user.click(screen.getByRole("radio", {name: "Two profile files"}));
        expect(screen.queryByRole("status")).not.toBeInTheDocument();
    });
});

describe("in Spanish", () => {
    it("words the groups, summary, actions and errors", async () => {
        const user = await open("es");
        await user.click(screen.getByRole("button", {name: "Elegir un archivo…"}));
        expect(await screen.findByRole("heading", {name: "Solo en este PC (1)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Solo en el archivo (2)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "En ambos (1)"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {name: "Desconocidas para este catálogo (1)"})).toBeInTheDocument();
        expect(screen.getByRole("status")).toHaveTextContent("En ambos: 1. Solo con este PC: 1. Solo con el archivo «Laptop»: 2. Fuera de este catálogo: 1.");
        expect(screen.getByRole("button", {name: /Instalar las apps que faltan \(2\)/})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Guardar el archivo como perfil"})).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Añadir al perfil (1)"})).toBeInTheDocument();
        api.CompareFile.mockRejectedValue(new Error("compare:too-big"));
        await user.click(screen.getByRole("button", {name: "Elegir un archivo…"}));
        expect(await screen.findByRole("alert")).toHaveTextContent("Eso es demasiado grande para ser un perfil.");
    });
});

describe("a comparison outlives the page", () => {
    const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));
    const tasks = () => within(screen.getByRole("heading", {name: "Tasks", level: 2}).closest("section") as HTMLElement);

    it("stays listed as running after leaving and coming back, blocks a second one, then shows the summary", async () => {
        let finish!: (r: CompareResult) => void;
        api.CompareFile.mockReturnValue(new Promise<CompareResult>((res) => { finish = res; }));
        const user = await open();
        await chooseFile(user);
        expect(await tasks().findByText("Comparing profiles: laptop.winforge.json")).toBeInTheDocument();
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(screen.queryByRole("heading", {level: 1, name: "Compare setups"})).not.toBeInTheDocument());
        await user.click(nav().getByRole("button", {name: /Profiles/}));
        // The compare view is still open where it was left.
        expect(await screen.findByRole("heading", {level: 1, name: "Compare setups"})).toBeInTheDocument();
        expect(tasks().getByText("Comparing profiles: laptop.winforge.json")).toBeInTheDocument();
        expect(tasks().getByText("Running")).toBeInTheDocument();
        expect(screen.getByRole("button", {name: "Choose a file…"})).toBeDisabled();
        expect(api.CompareFile).toHaveBeenCalledTimes(1);
        finish(result());
        await waitFor(() => expect(tasks().getAllByText("Done").length).toBeGreaterThan(0));
        expect(tasks().getByText(/In both: 1./)).toBeInTheDocument();
        expect(api.CompareFile).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(screen.getByRole("button", {name: "Choose a file…"})).toBeEnabled());
        // The answer that came in after coming back is on the page too.
        expect(await screen.findByRole("heading", {name: "Only on this PC (1)"})).toBeInTheDocument();
    });

    it("lists a comparison that failed", async () => {
        api.CompareFile.mockRejectedValue(new Error("compare:not-json"));
        const user = await open();
        await chooseFile(user);
        await waitFor(() => expect(tasks().getAllByText("Failed").length).toBeGreaterThan(0));
        expect(tasks().getByText("Comparing profiles: laptop.winforge.json")).toBeInTheDocument();
    });
});
