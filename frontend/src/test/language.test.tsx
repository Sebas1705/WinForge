import {render, screen, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {resetApi} from "./apiMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
});

describe("language", () => {
    it("repaints already-rendered catalog cards when the language changes", async () => {
        const user = userEvent.setup();
        render(<App/>);
        await screen.findByText(/catalog apps found/);
        await user.click(within(screen.getByRole("navigation", {name: "Main menu"})).getByRole("button", {name: /Catalog/}));
        expect(await screen.findByText("Fast, private web browser")).toBeInTheDocument();
        await user.click(screen.getByRole("button", {name: "Appearance and language"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Español"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "Listo"}));
        // Cards are memoised; they must still switch to the Spanish tagline.
        expect(await screen.findByText("Navegador rápido y privado")).toBeInTheDocument();
        expect(screen.queryByText("Fast, private web browser")).not.toBeInTheDocument();
    });
});
