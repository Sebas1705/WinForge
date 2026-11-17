import {act, render, screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useEffect} from "react";
import {describe, expect, it} from "vitest";
import {usePageState} from "../lib/pageState";

let setLater: ((v: number) => void) | null = null;
function Counter(p: { id: string }) {
    const [n, setN] = usePageState(`test:${p.id}`, 0);
    useEffect(() => { setLater = setN as (v: number) => void; });
    return <button onClick={() => setN((x) => x + 1)}>count {n}</button>;
}

describe("usePageState", () => {
    it("keeps the value when the component is unmounted and mounted again", async () => {
        const user = userEvent.setup();
        const a = render(<Counter id="a"/>);
        await user.click(screen.getByRole("button"));
        await user.click(screen.getByRole("button"));
        a.unmount();
        render(<Counter id="a"/>);
        expect(screen.getByRole("button", {name: "count 2"})).toBeInTheDocument();
    });

    it("keeps separate keys apart", () => {
        render(<><Counter id="b"/><Counter id="c"/></>);
        expect(screen.getAllByRole("button", {name: "count 0"})).toHaveLength(2);
    });

    it("records a value set after the component is gone, for the next mount", () => {
        const a = render(<Counter id="d"/>);
        a.unmount();
        act(() => setLater?.(7));
        render(<Counter id="d"/>);
        expect(screen.getByRole("button", {name: "count 7"})).toBeInTheDocument();
    });
});
