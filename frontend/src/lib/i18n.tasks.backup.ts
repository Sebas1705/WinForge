// Titles and results of the jobs the Backup page registers in the task list. Spanish must have exactly the English keys.

export const tasksBackupEn = {
    "tasks.backup.create": "Creating the backup…",
    "tasks.backup.rebuild": "Rebuilding this PC from a backup…",
    "tasks.backup.stage.apps": "Reinstalling apps…",
    "tasks.backup.stage.settings": "Restoring settings…",
    "tasks.backup.stage.folders": "Restoring folders…",
};

export const tasksBackupEs: Record<keyof typeof tasksBackupEn, string> = {
    "tasks.backup.create": "Creando la copia de seguridad…",
    "tasks.backup.rebuild": "Reconstruyendo este PC desde una copia…",
    "tasks.backup.stage.apps": "Reinstalando las apps…",
    "tasks.backup.stage.settings": "Restaurando los ajustes…",
    "tasks.backup.stage.folders": "Restaurando las carpetas…",
};
