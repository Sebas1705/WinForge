// Titles and results of the jobs the PC health page registers in the task list. Spanish must have exactly the English keys.

export const tasksHealthEn = {
    "tasks.health.scan": "Scanning PC health…",
    "tasks.health.updates": "Checking for Windows and driver updates…",
    "tasks.health.sum.scan": "{ok} of {total} checks are fine.",
    "tasks.health.sum.updates": "{n} updates found.",
};

export const tasksHealthEs: Record<keyof typeof tasksHealthEn, string> = {
    "tasks.health.scan": "Analizando la salud del PC…",
    "tasks.health.updates": "Buscando actualizaciones de Windows y de controladores…",
    "tasks.health.sum.scan": "{ok} de {total} comprobaciones están bien.",
    "tasks.health.sum.updates": "{n} actualizaciones encontradas.",
};
