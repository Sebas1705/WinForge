// Names of the sub-categories shown under a category in the catalog
// ("gaming/emulators" -> Emulators). Sub-categories without a line here fall
// back to their id, so adding one to the catalog never breaks the screen.

export const subEn = {
    "sub.other": "Other",
    "sub.gaming.launchers": "Stores, launchers and tools",
    "sub.gaming.emulators": "Emulators",
    "sub.gaming.mods": "Mods and modding tools",
    "sub.gaming.engines": "Open-source games and engines",
    "sub.dev.vcs": "Version control",
    "sub.dev.editors": "Editors and IDEs",
    "sub.dev.languages": "Languages",
    "sub.dev.cloud": "Cloud",
    "sub.dev.database": "Databases",
    "sub.dev.containers": "Containers",
    "sub.dev.build": "Build tools",
    "sub.dev.cli": "Command line",
    "sub.dev.terminal": "Terminals",
    "sub.dev.api": "APIs",
    "sub.dev.analysis": "Analysis and debugging",
    "sub.dev.network": "Network",
} as const;

export const subEs: Record<keyof typeof subEn, string> = {
    "sub.other": "Otros",
    "sub.gaming.launchers": "Tiendas, lanzadores y herramientas",
    "sub.gaming.emulators": "Emuladores",
    "sub.gaming.mods": "Mods y herramientas de mods",
    "sub.gaming.engines": "Juegos libres y motores",
    "sub.dev.vcs": "Control de versiones",
    "sub.dev.editors": "Editores e IDEs",
    "sub.dev.languages": "Lenguajes",
    "sub.dev.cloud": "Nube",
    "sub.dev.database": "Bases de datos",
    "sub.dev.containers": "Contenedores",
    "sub.dev.build": "Compilación",
    "sub.dev.cli": "Línea de comandos",
    "sub.dev.terminal": "Terminales",
    "sub.dev.api": "APIs",
    "sub.dev.analysis": "Análisis y depuración",
    "sub.dev.network": "Red",
};
