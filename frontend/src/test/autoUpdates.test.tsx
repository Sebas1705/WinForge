import {render, screen, waitFor, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach, describe, expect, it, vi} from "vitest";
import App from "../App";
import {autoError, dayName, summaryParts, upToDate, whenText} from "../lib/auto";
import {DICTS, KEYS, setLang} from "../lib/i18n";
import {api, resetApi} from "./apiMock";
import {autoOff, autoOn, autoRun} from "./autoMock";

vi.mock("../api", async () => (await import("./apiMock")).moduleMock);

beforeEach(() => {
    localStorage.setItem("winforge.tour", "1");
    resetApi();
    api.Upgrades.mockResolvedValue([]);
});

type Lang = "en" | "es";
const W: Record<Lang, {menu: string; updates: RegExp; found: RegExp}> = {
    en: {menu: "Main menu", updates: /Updates/, found: /catalog apps found/},
    es: {menu: "Menú principal", updates: /Actualizaciones/, found: /apps del catálogo/},
};

async function openUpdates(lang: Lang = "en") {
    if (lang === "es") localStorage.setItem("winforge.settings", JSON.stringify({language: "es"}));
    const user = userEvent.setup();
    render(<App/>);
    await screen.findByText(W[lang].found);
    await user.click(within(screen.getByRole("navigation", {name: W[lang].menu})).getByRole("button", {name: W[lang].updates}));
    await screen.findByRole("heading", {name: lang === "es" ? "Actualizaciones automáticas" : "Automatic updates"});
    return user;
}

const card = () => within(screen.getByRole("region", {name: /Automatic updates|Actualizaciones automáticas/}));

describe("automatic updates: off", () => {
    it("says it is off, and keeps the schedule choices hidden until asked", async () => {
        await openUpdates();
        expect(await screen.findByText("Off. WinForge only looks for updates when you ask.")).toBeInTheDocument();
        expect(screen.queryByRole("radio")).not.toBeInTheDocument();
        const setup = card().getByRole("button", {name: "Set up…"});
        expect(setup).toHaveAttribute("aria-expanded", "false");
        expect(setup).toHaveAttribute("aria-controls", "auto-form");
        expect(card().queryByRole("button", {name: "Turn off"})).not.toBeInTheDocument();
    });

    it("turns on with the safe defaults (Tell me, every day, 09:00) without asking anything", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        expect(card().getByRole("radio", {name: /Tell me/})).toBeChecked();
        expect(card().getByRole("radio", {name: /Every day/})).toBeChecked();
        expect(card().getByLabelText("Time")).toHaveValue("09:00");
        await user.click(card().getByRole("button", {name: "Turn on"}));
        expect(api.AutoEnable).toHaveBeenCalledWith({mode: "notify", frequency: "daily", weekday: 0, time: "09:00"}, "en");
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(await card().findByText("On: Tell me, every day at 09:00.")).toBeInTheDocument();
        expect(card().getByRole("button", {name: "Change…"})).toHaveAttribute("aria-expanded", "false");
        expect(card().getByRole("button", {name: "Turn off"})).toBeInTheDocument();
    });

    it("sends the chosen weekday and time, and only offers the weekday for a weekly schedule", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        expect(card().queryByLabelText("Day of the week")).not.toBeInTheDocument();
        await user.click(card().getByRole("radio", {name: /Every week/}));
        const day = card().getByLabelText("Day of the week");
        expect(within(day).getAllByRole("option").map((o) => o.textContent)).toEqual(["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]);
        await user.selectOptions(day, "Friday");
        const time = card().getByLabelText("Time");
        await user.clear(time);
        await user.type(time, "18:45");
        await user.click(card().getByRole("button", {name: "Turn on"}));
        expect(api.AutoEnable).toHaveBeenCalledWith({mode: "notify", frequency: "weekly", weekday: 5, time: "18:45"}, "en");
    });

    it("cannot be turned on without a valid time", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        await user.clear(card().getByLabelText("Time"));
        expect(card().getByRole("button", {name: "Turn on"})).toBeDisabled();
        expect(card().getByLabelText("Time")).toBeInvalid();
        expect(api.AutoEnable).not.toHaveBeenCalled();
    });

    it("says so, instead of offering a broken task, when there is no program to schedule", async () => {
        api.AutoState.mockResolvedValue(autoOff({canEnable: false, problem: "runner-missing"}));
        await openUpdates();
        expect(await card().findByRole("alert")).toHaveTextContent(/cannot find the program the schedule has to start/);
        expect(card().getByRole("button", {name: "Set up…"})).toBeDisabled();
    });

    it("cancelling the form changes nothing and returns focus to the button", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        await user.click(card().getByRole("radio", {name: /Update for me/}));
        await user.click(card().getByRole("button", {name: "Cancel"}));
        expect(card().queryByRole("radio")).not.toBeInTheDocument();
        expect(card().getByRole("button", {name: "Set up…"})).toHaveFocus();
        expect(api.AutoEnable).not.toHaveBeenCalled();
    });
});

describe("automatic updates: Update for me asks first", () => {
    async function chooseApply() {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        await user.click(card().getByRole("radio", {name: /Update for me/}));
        await user.click(card().getByRole("button", {name: "Turn on"}));
        return user;
    }

    it("says exactly what will happen unattended, and does nothing until confirmed", async () => {
        await chooseApply();
        const dialog = within(await screen.findByRole("alertdialog", {name: "Update apps on their own?"}));
        expect(api.AutoEnable).not.toHaveBeenCalled();
        expect(dialog.getByText(/From now on, every day at 09:00, WinForge will install updates by itself/)).toBeInTheDocument();
        for (const line of [
            /only updates apps of the WinForge catalog\. It never runs scripts \(recipes\) and never updates WinForge itself/,
            /ignored, paused or skipped/,
            /needs administrator rights/,
            /never restarts the PC/,
            /log you can read on this page/,
            /turn it off at any time/,
        ]) expect(dialog.getByText(line)).toBeInTheDocument();
        expect(dialog.getByRole("button", {name: "Cancel"})).toHaveFocus(); // the safe answer is the default
    });

    it("cancel and Escape leave it as it was", async () => {
        const user = await chooseApply();
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Cancel"}));
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        await user.click(card().getByRole("button", {name: "Turn on"}));
        await screen.findByRole("alertdialog");
        await user.keyboard("{Escape}");
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(api.AutoEnable).not.toHaveBeenCalled();
    });

    it("confirming turns it on in apply mode", async () => {
        const user = await chooseApply();
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Update for me"}));
        await waitFor(() => expect(api.AutoEnable).toHaveBeenCalledWith({mode: "apply", frequency: "daily", weekday: 0, time: "09:00"}, "en"));
        expect(await card().findByText("On: Update for me, every day at 09:00.")).toBeInTheDocument();
    });

    it("asks when an existing Tell me schedule is changed to Update for me", async () => {
        api.AutoState.mockResolvedValue(autoOn({mode: "notify"}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Change…"}));
        await user.click(card().getByRole("radio", {name: /Update for me/}));
        await user.click(card().getByRole("button", {name: "Save changes"}));
        await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", {name: "Cancel"}));
        expect(api.AutoEnable).not.toHaveBeenCalled();
    });

    it("does not ask when an Update-for-me schedule only changes its time", async () => {
        api.AutoState.mockResolvedValue(autoOn({mode: "apply"}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Change…"}));
        const time = card().getByLabelText("Time");
        await user.clear(time);
        await user.type(time, "22:00");
        await user.click(card().getByRole("button", {name: "Save changes"}));
        expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
        expect(api.AutoEnable).toHaveBeenCalledWith({mode: "apply", frequency: "daily", weekday: 0, time: "22:00"}, "en");
    });
});

describe("automatic updates: on", () => {
    it("shows what is scheduled and when it runs next, and loads the form from the task", async () => {
        api.AutoState.mockResolvedValue(autoOn({mode: "apply", frequency: "weekly", weekday: 5, time: "18:00"}));
        const user = await openUpdates();
        expect(await card().findByText("On: Update for me, every Friday at 18:00.")).toBeInTheDocument();
        expect(card().getByText(/Next run:/)).toBeInTheDocument();
        await user.click(card().getByRole("button", {name: "Change…"}));
        expect(card().getByRole("radio", {name: /Update for me/})).toBeChecked();
        expect(card().getByRole("radio", {name: /Every week/})).toBeChecked();
        expect(card().getByLabelText("Day of the week")).toHaveValue("5");
        expect(card().getByLabelText("Time")).toHaveValue("18:00");
    });

    it("turning off removes the task and says the log is kept", async () => {
        api.AutoState.mockResolvedValue(autoOn({runs: [autoRun()]}));
        api.AutoDisable.mockResolvedValue(autoOff({runs: [autoRun()]}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Turn off"}));
        expect(api.AutoDisable).toHaveBeenCalledTimes(1);
        expect(await card().findByText("Off. WinForge only looks for updates when you ask.")).toBeInTheDocument();
        expect(card().getAllByText("Automatic updates are off. The log of past runs is kept.").length).toBeGreaterThan(0);
        expect(card().queryByRole("button", {name: "Turn off"})).not.toBeInTheDocument();
        expect(card().getByRole("button", {name: /Last runs \(1\)/})).toBeInTheDocument();
    });

    it("warns about a task Windows has switched off, one edited elsewhere and one whose program is gone", async () => {
        api.AutoState.mockResolvedValue(autoOn({enabled: false, foreign: true, problem: "task-program-missing"}));
        await openUpdates();
        expect(await card().findByText(/Windows has this task switched off/)).toBeInTheDocument();
        expect(card().getByText(/changed outside WinForge/)).toBeInTheDocument();
        expect(card().getByRole("alert")).toHaveTextContent(/no longer there/);
        expect(card().queryByText(/Next run:/)).not.toBeInTheDocument();
    });
});

describe("automatic updates: Check now", () => {
    it("shows what a dry check found, with every app and what would happen to it", async () => {
        api.AutoRunNow.mockResolvedValue(autoRun({scheduled: false, status: "attention", items: [
            {id: "git", name: "Git", from: "1.0", to: "2.0", result: "found"},
            {id: "7zip", name: "7-Zip", from: "23", to: "24", result: "needs-admin"},
            {id: "vlc", name: "VLC", from: "3.0", to: "3.1", result: "ignored", reason: "always"},
        ]}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        const result = within(await screen.findByRole("region", {name: "Result of this check"}));
        const rows = result.getAllByRole("listitem").map((r) => r.textContent);
        expect(rows).toEqual([
            expect.stringContaining("Git 1.0 → 2.0Update available"),
            expect.stringContaining("7-Zip 23 → 24Needs administrator rights"),
            expect.stringContaining("VLC 3.0 → 3.1Ignored (never updated)"),
        ]);
        expect(api.AutoEnable).not.toHaveBeenCalled();
    });

    it("says everything is up to date", async () => {
        api.AutoRunNow.mockResolvedValue(autoRun({scheduled: false}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        expect(await within(await screen.findByRole("region", {name: "Result of this check"})).findByText("Everything is up to date.")).toBeInTheDocument();
    });

    it("words a run-level problem in the person's language", async () => {
        api.AutoRunNow.mockResolvedValue(autoRun({scheduled: false, status: "error", error: "settings-damaged"}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        expect(await screen.findByText(/could not read your list of ignored updates, so it touched nothing/, {selector: ".autowarn"})).toBeInTheDocument();
    });

    it("words a Go error code, never the text Go sent", async () => {
        api.AutoRunNow.mockRejectedValue(new Error("auto:busy"));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        expect(await card().findByRole("alert")).toHaveTextContent("Another check is running right now. Try again in a moment.");
    });

    it("is disabled while it works", async () => {
        let done: (r: ReturnType<typeof autoRun>) => void = () => {};
        api.AutoRunNow.mockImplementation(() => new Promise((res) => { done = res; }));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        expect(card().getByRole("button", {name: "Check now"})).toBeDisabled();
        done(autoRun());
        await waitFor(() => expect(card().getByRole("button", {name: "Check now"})).toBeEnabled());
    });
});

describe("automatic updates: last runs", () => {
    const runs = [
        autoRun({id: "b", started: "2026-11-16T09:30:00Z", mode: "apply", status: "partial", items: [
            {id: "git", name: "Git", from: "1.0", to: "2.0", result: "updated", reboot: true},
            {id: "vlc", name: "VLC", from: "3.0", to: "3.1", result: "failed", reason: "network"},
            {id: "7zip", name: "7-Zip", from: "23", to: "24", result: "needs-admin"},
        ]}),
        autoRun({id: "a", started: "2026-11-15T09:30:00Z", status: "error", error: "no-winget"}),
        autoRun({id: "c", started: "2026-11-14T09:30:00Z", status: "ok"}),
    ];

    it("lists the runs newest first, with what happened, and is closed until opened", async () => {
        api.AutoState.mockResolvedValue(autoOn({runs}));
        const user = await openUpdates();
        const toggle = await card().findByRole("button", {name: "Last runs (3)"});
        expect(toggle).toHaveAttribute("aria-expanded", "false");
        expect(card().queryByRole("list", {name: "Last automatic runs"})).not.toBeInTheDocument();
        await user.click(toggle);
        const items = within(card().getByRole("list", {name: "Last automatic runs"})).getAllByRole("listitem", {}).filter((li) => li.classList.contains("autorun"));
        expect(items).toHaveLength(3);
        expect(items[0]).toHaveTextContent("Some failed");
        expect(items[0]).toHaveTextContent("1 updated · 1 need you · 1 failed");
        expect(items[1]).toHaveTextContent("Could not run");
        expect(items[1]).toHaveTextContent("winget is not available on this PC.");
        expect(items[2]).toHaveTextContent("Done");
        expect(items[2]).toHaveTextContent("Nothing to update");
    });

    it("opens the versions before and after, the reason for a failure and the restart note", async () => {
        api.AutoState.mockResolvedValue(autoOn({runs}));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Last runs (3)"}));
        const show = card().getAllByRole("button", {name: /^Show what happened in the run of/})[0];
        expect(show).toHaveAttribute("aria-expanded", "false");
        await user.click(show);
        const run = within(card().getAllByRole("listitem").find((li) => li.classList.contains("autorun")) as HTMLElement);
        const rows = run.getAllByRole("listitem").map((r) => r.textContent ?? "");
        expect(rows[0]).toContain("Git 1.0 → 2.0Updated");
        expect(rows[0]).toContain("WinForge never restarts the PC for you");
        expect(rows[1]).toContain("VLC 3.0 → 3.1Failed (the download failed)");
        expect(rows[2]).toContain("7-Zip 23 → 24Needs administrator rights");
        expect(run.getByRole("button", {name: /^Hide the details of the run of/})).toHaveAttribute("aria-expanded", "true");
    });

    it("says there is nothing yet when there are no runs", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Last runs (0)"}));
        expect(card().getByText(/No scheduled run yet/)).toBeInTheDocument();
    });
});

describe("automatic updates: errors from Go are codes that become sentences", () => {
    it("maps each code and keeps what Windows said about a refused task", async () => {
        setLang("en");
        expect(autoError(new Error("auto:runner-missing"))).toMatch(/cannot find the program/);
        expect(autoError(new Error("auto:invalid: time \"x\""))).toBe("Those choices are not valid. Check the time and try again.");
        expect(autoError(new Error("auto:unsupported"))).toBe("Automatic updates are only available on Windows.");
        expect(autoError(new Error("auto:task-failed (exit 1) Access is denied."))).toBe("Windows would not accept the change to the scheduled task. ((exit 1) Access is denied.)");
        expect(autoError(new Error("something else"))).toBe("something else");
        setLang("es");
        expect(autoError(new Error("auto:busy"))).toBe("Ya hay otra búsqueda en marcha. Inténtalo de nuevo en un momento.");
        setLang("en");
    });

    it("shows the sentence when turning on is refused", async () => {
        api.AutoEnable.mockRejectedValue(new Error("auto:task-failed (exit 1) Access is denied."));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        await user.click(card().getByRole("button", {name: "Turn on"}));
        expect(await card().findByRole("alert")).toHaveTextContent(/Windows would not accept the change to the scheduled task/);
        expect(card().getByRole("button", {name: "Turn on"})).toBeEnabled(); // the form stays open to try again
    });
});

describe("automatic updates: Spanish", () => {
    it("is entirely in Spanish and sends the Spanish language for the notification", async () => {
        api.AutoState.mockResolvedValue(autoOff());
        const user = await openUpdates("es");
        expect(await card().findByText("Desactivadas. WinForge solo busca actualizaciones cuando se lo pides.")).toBeInTheDocument();
        await user.click(card().getByRole("button", {name: "Configurar…"}));
        expect(card().getByRole("group", {name: "Qué debe hacer"})).toBeInTheDocument();
        expect(card().getByRole("group", {name: "Con qué frecuencia"})).toBeInTheDocument();
        await user.click(card().getByRole("radio", {name: /Cada semana/}));
        expect(within(card().getByLabelText("Día de la semana")).getAllByRole("option").map((o) => o.textContent)[0]).toBe("Lunes");
        await user.click(card().getByRole("radio", {name: /Actualizar por mí/}));
        await user.click(card().getByRole("button", {name: "Activar"}));
        const dialog = within(await screen.findByRole("alertdialog", {name: "¿Actualizar las aplicaciones por tu cuenta?"}));
        expect(dialog.getByText(/A partir de ahora, cada lunes a las 09:00/)).toBeInTheDocument();
        await user.click(dialog.getByRole("button", {name: "Actualizar por mí"}));
        await waitFor(() => expect(api.AutoEnable).toHaveBeenCalledWith({mode: "apply", frequency: "weekly", weekday: 1, time: "09:00"}, "es"));
        expect(await card().findByText("Activadas: Actualizar por mí, cada lunes a las 09:00.")).toBeInTheDocument();
    });

    it("words runs and checks in Spanish", async () => {
        api.AutoState.mockResolvedValue(autoOn({runs: [autoRun({mode: "apply", status: "partial", items: [
            {id: "git", name: "Git", from: "1", to: "2", result: "updated"},
            {id: "vlc", name: "VLC", from: "3", to: "4", result: "failed", reason: "disk"},
        ]})]}));
        api.AutoRunNow.mockResolvedValue(autoRun({scheduled: false, status: "attention", items: [{id: "git", name: "Git", from: "1", to: "2", result: "found"}]}));
        const user = await openUpdates("es");
        await user.click(await card().findByRole("button", {name: "Últimas ejecuciones (1)"}));
        expect(card().getByText("Algunas fallaron")).toBeInTheDocument();
        expect(card().getByText(/1 actualizadas · 1 fallaron/)).toBeInTheDocument();
        await user.click(card().getByRole("button", {name: /^Mostrar qué pasó en la ejecución del/}));
        expect(card().getByText(/Falló/, {selector: ".autoresult"})).toHaveTextContent("no hay espacio suficiente en el disco");
        await user.click(card().getByRole("button", {name: "Buscar ahora"}));
        expect(await within(await screen.findByRole("region", {name: "Resultado de esta búsqueda"})).findByText("Actualización disponible")).toBeInTheDocument();
    });

    it("has every English key in Spanish and nothing left over", () => {
        const auto = KEYS.filter((k) => k.startsWith("auto."));
        expect(auto.length).toBeGreaterThan(80);
        for (const k of auto) {
            expect(DICTS.es[k], k).toBeTruthy();
            // placeholders must agree, or a sentence would print "{name}"
            const holes = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(",");
            expect(holes(DICTS.es[k]), k).toBe(holes(DICTS.en[k]));
        }
    });
});

describe("automatic updates: the app side of the notification", () => {
    it("opens the Updates page when started from the notification's link", async () => {
        api.LaunchTarget.mockResolvedValue("updates");
        render(<App/>);
        expect(await screen.findByRole("heading", {name: "Automatic updates"})).toBeInTheDocument();
        expect(screen.getByRole("heading", {level: 1, name: "Updates"})).toBeInTheDocument();
    });

    it("starts on Home as usual otherwise, and survives a failing call", async () => {
        api.LaunchTarget.mockRejectedValue(new Error("not bound yet"));
        api.AutoSetLanguage.mockRejectedValue(new Error("not bound yet"));
        render(<App/>);
        expect(await screen.findByText(/catalog apps found/)).toBeInTheDocument();
        expect(screen.queryByRole("heading", {name: "Automatic updates"})).not.toBeInTheDocument();
    });

    it("keeps the notification's language in step with the interface language", async () => {
        localStorage.setItem("winforge.settings", JSON.stringify({language: "es"}));
        render(<App/>);
        await screen.findByText(W.es.found);
        await waitFor(() => expect(api.AutoSetLanguage).toHaveBeenCalledWith("es"));
        const user = userEvent.setup();
        await user.click(screen.getByRole("button", {name: "Apariencia e idioma"}));
        await user.click(within(screen.getByRole("dialog")).getByRole("button", {name: "English"}));
        await waitFor(() => expect(api.AutoSetLanguage).toHaveBeenLastCalledWith("en"));
    });
});

describe("automatic updates: helpers", () => {
    it("words the schedule and the counts", () => {
        setLang("en");
        expect(whenText({frequency: "daily", weekday: 0, time: "07:05"})).toBe("every day at 07:05");
        expect(whenText({frequency: "weekly", weekday: 0, time: "07:05"})).toBe("every Sunday at 07:05");
        expect(dayName(6)).toBe("Saturday");
        expect(summaryParts([])).toEqual(["Nothing to update"]);
        expect(summaryParts([{id: "a", name: "A", from: "1", to: "2", result: "ignored"}])).toEqual(["1 ignored"]);
        expect(upToDate(autoRun({items: [{id: "a", name: "A", from: "1", to: "2", result: "ignored"}]}))).toBe(true);
        expect(upToDate(autoRun({items: [{id: "a", name: "A", from: "1", to: "2", result: "needs-admin"}]}))).toBe(false);
        expect(upToDate(autoRun({error: "timeout"}))).toBe(false);
    });
});

describe("automatic updates: jobs outlive the page", () => {
    const nav = () => within(screen.getByRole("navigation", {name: "Main menu"}));
    const tasks = () => within(screen.getByRole("heading", {name: "Tasks", level: 3}).closest("section") as HTMLElement);

    it("keeps a running check in the Tasks list after leaving and coming back, blocks a second one, then shows the result", async () => {
        let done: (r: ReturnType<typeof autoRun>) => void = () => {};
        api.AutoRunNow.mockImplementation(() => new Promise((res) => { done = res; }));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        await user.click(nav().getByRole("button", {name: /Home/}));
        await waitFor(() => expect(screen.queryByRole("heading", {name: "Automatic updates"})).not.toBeInTheDocument());
        await user.click(nav().getByRole("button", {name: /Updates/}));
        await screen.findByRole("heading", {name: "Automatic updates"});
        expect(tasks().getByText("Checking for updates now")).toBeInTheDocument();
        expect(tasks().getByText("Running")).toBeInTheDocument();
        expect(tasks().queryByRole("button", {name: /^Stop/})).not.toBeInTheDocument(); // the check cannot be stopped from here
        expect(await card().findByRole("button", {name: "Check now"})).toBeDisabled();
        expect(api.AutoRunNow).toHaveBeenCalledTimes(1);
        done(autoRun({scheduled: false}));
        await waitFor(() => expect(tasks().getAllByText("Done").length).toBeGreaterThan(0));
        expect(tasks().getByText("Everything is up to date.")).toBeInTheDocument();
        expect(api.AutoRunNow).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(card().getByRole("button", {name: "Check now"})).toBeEnabled());
    });

    it("locks saving while a check runs and lists a failed check", async () => {
        api.AutoRunNow.mockRejectedValue(new Error("auto:busy"));
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Check now"}));
        await waitFor(() => expect(tasks().getAllByText("Failed").length).toBeGreaterThan(0));
        expect(tasks().getByText("Another check is running right now. Try again in a moment.")).toBeInTheDocument();
    });

    it("lists saving the schedule as a task", async () => {
        const user = await openUpdates();
        await user.click(await card().findByRole("button", {name: "Set up…"}));
        await user.click(card().getByRole("button", {name: "Turn on"}));
        await waitFor(() => expect(tasks().getAllByText("Done").length).toBeGreaterThan(0));
        expect(tasks().getByText("Saving the update schedule")).toBeInTheDocument();
    });
});
