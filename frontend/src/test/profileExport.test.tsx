import {render, screen, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {api, resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

async function openMine(detail: "simple" | "advanced") {
    localStorage.setItem("winforge.settings", JSON.stringify({detail}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(/catalog apps found/);
    await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Profiles/}));
    await user.click(await screen.findByRole("button", {name: /Mine/}));
    await screen.findByRole("button", {name: /All profiles/});
    await user.click(screen.getByRole("button", {name: /More options/}));
    return user;
}

describe("exporting a profile for winget configure", () => {
    it("is offered in advanced mode, next to the other exports", async () => {
        await openMine("advanced");
        const menu = within(screen.getByRole("menu"));
        const item = menu.getByRole("menuitem", {name: "Export for winget configure"});
        expect(item).toHaveAttribute("title", expect.stringContaining("recipes run scripts"));
        expect(menu.getByRole("menuitem", {name: "Export for winget"})).toBeInTheDocument();
    });

    it("is not offered in simple mode, which hides the technical exports", async () => {
        await openMine("simple");
        expect(screen.queryByRole("menuitem", {name: /winget configure/})).not.toBeInTheDocument();
    });

    it("asks the backend for the profile that is open and says where it was saved", async () => {
        api.ExportConfigure.mockResolvedValue("D:\\setups\\mine.winget");
        const user = await openMine("advanced");
        await user.click(screen.getByRole("menuitem", {name: "Export for winget configure"}));
        expect(api.ExportConfigure).toHaveBeenCalledTimes(1);
        expect(api.ExportConfigure.mock.calls[0][0]).toEqual(expect.objectContaining({id: "mine"}));
        expect(await screen.findByText(/D:\\setups\\mine\.winget/)).toBeInTheDocument();
        expect(screen.queryByRole("menu")).not.toBeInTheDocument(); // the menu closes
    });

    it("stays quiet when the save dialog is cancelled, and shows a real failure", async () => {
        api.ExportConfigure.mockResolvedValueOnce("");
        const user = await openMine("advanced");
        await user.click(screen.getByRole("menuitem", {name: "Export for winget configure"}));
        expect(screen.queryByText(/winget/i, {selector: "[role=status]"})).not.toBeInTheDocument();
        api.ExportConfigure.mockRejectedValueOnce(new Error("access is denied"));
        await user.click(screen.getByRole("button", {name: /More options/}));
        await user.click(screen.getByRole("menuitem", {name: "Export for winget configure"}));
        expect(await screen.findByText("access is denied")).toBeInTheDocument();
    });

    it("speaks Spanish", async () => {
        localStorage.setItem("winforge.settings", JSON.stringify({detail: "advanced", language: "es"}));
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/apps del catálogo encontradas/);
        await user.click(within(screen.getByRole("navigation", {name: "Menú principal"})).getByRole("button", {name: /Perfiles/}));
        await user.click(await screen.findByRole("button", {name: /Mine/}));
        await user.click(await screen.findByRole("button", {name: /Más opciones/}));
        expect(screen.getByRole("menuitem", {name: "Exportar para winget configure"})).toBeInTheDocument();
    });
});
