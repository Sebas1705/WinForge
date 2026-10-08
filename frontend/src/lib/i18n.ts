import {backupEn, backupEs} from "./i18n.backup";
import {gamesEn, gamesEs} from "./i18n.games";
import {subEn, subEs} from "./i18n.sub";
import {healthEn, healthEs} from "./i18n.health";
import {uiEn, uiEs} from "./i18n.ui";

// Interface strings in English and Spanish. Catalog content (app names,
// descriptions, profile names) comes from the catalog and stays as written
// there; only the interface is translated.

const baseEn = {
    "nav.home": "Home",
    "nav.profiles": "Profiles",
    "nav.catalog": "Catalog",
    "nav.updates": "Updates",
    "common.cancel": "Cancel",
    "common.close": "Close",
    "common.save": "Save",
    "common.done": "Done",
    "common.clear": "Clear",
    "common.install": "Install",
    "rail.rescan": "Rescan PC",
    "rail.scanning": "Scanning…",
    "rail.appearance": "Appearance and language",
    "status.admin": "administrator",
    "status.user": "standard user",
    "status.userHint": "Some installers and setup steps need administrator rights",
    "wingetOnlyDetect": "Detection works, but installing needs winget.",

    "home.eyebrow": "This PC",
    "home.foundOf": "of {total} catalog apps found",
    "home.sub": "WinForge reads winget, the registry and your PATH.",
    "home.ruler": "Every catalog app; lit when installed",
    "home.next": "Closest to done",
    "home.nextEmpty": "Nothing is half-installed yet. Pick a profile to start.",
    "home.missing": "{n} missing",
    "home.categories": "By category",
    "home.updates": "Updates",
    "home.updatesCheck": "Check for updates",
    "home.updatesWord": "apps can be updated",
    "home.updatesNone": "Everything is up to date.",
    "home.updatesView": "View updates",

    "profiles.search": "Search profiles",
    "profiles.dev": "Developer",
    "profiles.general": "General",
    "profiles.mine": "Mine",
    "profiles.installMissing": "Install what’s missing…",
    "profiles.installN": "Install {n} selected…",
    "profiles.export": "Export",
    "profiles.exportWinget": "Export for winget",
    "profiles.exportWingetHint": "A file that `winget import` understands",
    "profiles.exportScript": "Export script (.ps1)",
    "profiles.exportScriptHint": "A readable PowerShell script for what is missing",
    "profiles.edit": "Edit in catalog",
    "profiles.delete": "Delete",
    "profiles.fromPC": "Save this PC as profile…",
    "profiles.import": "Import profile…",
    "profiles.progress": "{have} of {total} installed",
    "profiles.after": "After installing: {list}",
    "profiles.include": "Include {name}",
    "profiles.none": "No profile matches.",

    "catalog.search": "Search apps, publishers, winget ids…  ( / · Ctrl+K )",
    "catalog.all": "All",
    "catalog.state.all": "Any state",
    "catalog.state.installed": "Installed",
    "catalog.state.missing": "Not installed",
    "catalog.oss": "Open source",
    "catalog.sort.name": "Name",
    "catalog.sort.category": "Category",
    "catalog.sort.missing": "Not installed first",
    "catalog.shown": "{shown} of {total} shown",
    "catalog.selected": "{n} selected, {m} to install",
    "catalog.selectVisible": "Select visible",
    "catalog.saveProfile": "Save as profile…",
    "catalog.installN": "Install {n}…",
    "catalog.empty": "No app matches these filters.",
    "catalog.installed": "installed {v}",
    "badge.admin": "admin",
    "badge.oss": "open source",

    "updates.title": "Updates",
    "updates.hint": "Only apps from the catalog are listed; nothing else on this PC is touched.",
    "updates.check": "Check for updates",
    "updates.checking": "Asking winget…",
    "updates.none": "Everything from the catalog is up to date.",
    "updates.notChecked": "Check what has a newer version available.",
    "updates.updateN": "Update {n}…",
    "updates.all": "Update all",
    "updates.from": "{from} → {to}",

    "run.install": "Install “{name}”",
    "run.updateTitle": "Update apps",
    "run.installing": "Installing…",
    "run.done": "Done",
    "run.errors": "Finished with errors",
    "run.needsAdmin": "Some steps need administrator rights.",
    "run.restartAdmin": "Restart as administrator",
    "run.steps": "Run {n} steps",
    "run.stop": "Stop",
    "run.exportScript": "Export as script…",
    "run.copyLog": "Copy log",
    "run.copied": "Log copied.",
    "run.setup": "Setup",
    "run.update": "Update",
    "run.reboot": "Some apps may ask for a restart to finish.",
    "run.stepOf": "{done} of {total}",

    "settings.title": "Appearance and language",
    "settings.theme": "Theme",
    "settings.system": "System",
    "settings.dark": "Dark",
    "settings.light": "Light",
    "settings.accent": "Accent",
    "settings.density": "Density",
    "settings.comfortable": "Comfortable",
    "settings.compact": "Compact",
    "settings.language": "Language",
    "settings.auto": "Automatic",
    "settings.updates": "Updates",
    "settings.checkAtStart": "Check at startup",
    "settings.checkNow": "Check now",
    "settings.version": "Version {v}",

    "banner.available": "WinForge {v} is available (you have {c}).",
    "banner.now": "Update now",
    "banner.notes": "Release notes",
    "toast.upToDate": "WinForge is up to date.",
    "dlg.saveSelection": "Save selection as a profile",
    "dlg.saveFromPC": "Save this PC as a profile",
    "dlg.name": "Profile name",
    "dlg.save": "Save",
    "toast.saved": "Saved “{name}”.",
    "toast.savedPC": "Saved {n} installed apps as “{name}”.",
    "toast.imported": "Imported “{name}”.",
    "toast.importedDropped": "Imported “{name}”; {n} entries are not in this catalog and were ignored.",
    "toast.nothing": "Nothing to install: everything here is already on this PC.",
    "toast.savedTo": "Saved to {path}",
    "toast.deleted": "Deleted.",

    "detail.website": "Website",
    "detail.homepage": "Homepage",
    "detail.wingetId": "winget id",
    "detail.license": "License",
    "detail.requires": "Needs",
    "detail.detected": "Detected by",
    "detail.copyCommand": "Copy install command",
    "detail.copied": "Command copied.",

    "cat.ai": "AI",
    "cat.browsers": "Browsers",
    "cat.communication": "Communication",
    "cat.design": "Design and 3D",
    "cat.dev": "Development",
    "cat.education": "Education and science",
    "cat.gaming": "Gaming",
    "cat.media": "Media",
    "cat.network": "Network",
    "cat.productivity": "Productivity",
    "cat.runtimes": "Runtimes",
    "cat.security": "Security",
    "cat.system": "System",
    "cat.utilities": "Utilities",
} as const;

const en = {...baseEn, ...healthEn, ...uiEn, ...backupEn, ...subEn, ...gamesEn} as const;

export type Key = keyof typeof en;

const baseEs: Record<keyof typeof baseEn, string> = {
    "nav.home": "Inicio",
    "nav.profiles": "Perfiles",
    "nav.catalog": "Catálogo",
    "nav.updates": "Actualizaciones",
    "common.cancel": "Cancelar",
    "common.close": "Cerrar",
    "common.save": "Guardar",
    "common.done": "Listo",
    "common.clear": "Limpiar",
    "common.install": "Instalar",
    "rail.rescan": "Volver a escanear",
    "rail.scanning": "Escaneando…",
    "rail.appearance": "Apariencia e idioma",
    "status.admin": "administrador",
    "status.user": "usuario estándar",
    "status.userHint": "Algunos instaladores y pasos de configuración necesitan permisos de administrador",
    "wingetOnlyDetect": "La detección funciona, pero instalar requiere winget.",

    "home.eyebrow": "Este PC",
    "home.foundOf": "de {total} apps del catálogo encontradas",
    "home.sub": "WinForge lee winget, el registro y tu PATH.",
    "home.ruler": "Cada app del catálogo; encendida si está instalada",
    "home.next": "Más cerca de completarse",
    "home.nextEmpty": "Aún no hay nada a medias. Elige un perfil para empezar.",
    "home.missing": "faltan {n}",
    "home.categories": "Por categoría",
    "home.updates": "Actualizaciones",
    "home.updatesCheck": "Buscar actualizaciones",
    "home.updatesWord": "apps se pueden actualizar",
    "home.updatesNone": "Todo está al día.",
    "home.updatesView": "Ver actualizaciones",

    "profiles.search": "Buscar perfiles",
    "profiles.dev": "Desarrollo",
    "profiles.general": "General",
    "profiles.mine": "Mis perfiles",
    "profiles.installMissing": "Instalar lo que falta…",
    "profiles.installN": "Instalar {n} seleccionadas…",
    "profiles.export": "Exportar",
    "profiles.exportWinget": "Exportar para winget",
    "profiles.exportWingetHint": "Un archivo que entiende `winget import`",
    "profiles.exportScript": "Exportar script (.ps1)",
    "profiles.exportScriptHint": "Un script de PowerShell legible con lo que falta",
    "profiles.edit": "Editar en el catálogo",
    "profiles.delete": "Eliminar",
    "profiles.fromPC": "Guardar este PC como perfil…",
    "profiles.import": "Importar perfil…",
    "profiles.progress": "{have} de {total} instaladas",
    "profiles.after": "Después de instalar: {list}",
    "profiles.include": "Incluir {name}",
    "profiles.none": "Ningún perfil coincide.",

    "catalog.search": "Buscar apps, editores, ids de winget…  ( / · Ctrl+K )",
    "catalog.all": "Todas",
    "catalog.state.all": "Cualquier estado",
    "catalog.state.installed": "Instaladas",
    "catalog.state.missing": "Sin instalar",
    "catalog.oss": "Open source",
    "catalog.sort.name": "Nombre",
    "catalog.sort.category": "Categoría",
    "catalog.sort.missing": "Sin instalar primero",
    "catalog.shown": "{shown} de {total} visibles",
    "catalog.selected": "{n} seleccionadas, {m} por instalar",
    "catalog.selectVisible": "Seleccionar visibles",
    "catalog.saveProfile": "Guardar como perfil…",
    "catalog.installN": "Instalar {n}…",
    "catalog.empty": "Ninguna app coincide con estos filtros.",
    "catalog.installed": "instalada {v}",
    "badge.admin": "admin",
    "badge.oss": "open source",

    "updates.title": "Actualizaciones",
    "updates.hint": "Solo aparecen apps del catálogo; no se toca nada más de este PC.",
    "updates.check": "Buscar actualizaciones",
    "updates.checking": "Consultando a winget…",
    "updates.none": "Todo lo del catálogo está al día.",
    "updates.notChecked": "Comprueba qué apps tienen una versión nueva.",
    "updates.updateN": "Actualizar {n}…",
    "updates.all": "Actualizar todas",
    "updates.from": "{from} → {to}",

    "run.install": "Instalar «{name}»",
    "run.updateTitle": "Actualizar apps",
    "run.installing": "Instalando…",
    "run.done": "Terminado",
    "run.errors": "Terminó con errores",
    "run.needsAdmin": "Algunos pasos necesitan permisos de administrador.",
    "run.restartAdmin": "Reiniciar como administrador",
    "run.steps": "Ejecutar {n} pasos",
    "run.stop": "Detener",
    "run.exportScript": "Exportar como script…",
    "run.copyLog": "Copiar registro",
    "run.copied": "Registro copiado.",
    "run.setup": "Configuración",
    "run.update": "Actualizar",
    "run.reboot": "Algunas apps pueden pedir reiniciar para terminar.",
    "run.stepOf": "{done} de {total}",

    "settings.title": "Apariencia e idioma",
    "settings.theme": "Tema",
    "settings.system": "Sistema",
    "settings.dark": "Oscuro",
    "settings.light": "Claro",
    "settings.accent": "Acento",
    "settings.density": "Densidad",
    "settings.comfortable": "Cómoda",
    "settings.compact": "Compacta",
    "settings.language": "Idioma",
    "settings.auto": "Automático",
    "settings.updates": "Actualizaciones",
    "settings.checkAtStart": "Buscar al iniciar",
    "settings.checkNow": "Comprobar ahora",
    "settings.version": "Versión {v}",

    "banner.available": "WinForge {v} está disponible (tienes {c}).",
    "banner.now": "Actualizar ahora",
    "banner.notes": "Notas de la versión",
    "toast.upToDate": "WinForge está al día.",
    "dlg.saveSelection": "Guardar selección como perfil",
    "dlg.saveFromPC": "Guardar este PC como perfil",
    "dlg.name": "Nombre del perfil",
    "dlg.save": "Guardar",
    "toast.saved": "Guardado «{name}».",
    "toast.savedPC": "Guardadas {n} apps instaladas como «{name}».",
    "toast.imported": "Importado «{name}».",
    "toast.importedDropped": "Importado «{name}»; {n} entradas no están en este catálogo y se ignoraron.",
    "toast.nothing": "Nada que instalar: todo esto ya está en este PC.",
    "toast.savedTo": "Guardado en {path}",
    "toast.deleted": "Eliminado.",

    "detail.website": "Sitio web",
    "detail.homepage": "Página principal",
    "detail.wingetId": "id de winget",
    "detail.license": "Licencia",
    "detail.requires": "Necesita",
    "detail.detected": "Detectada por",
    "detail.copyCommand": "Copiar comando de instalación",
    "detail.copied": "Comando copiado.",

    "cat.ai": "IA",
    "cat.browsers": "Navegadores",
    "cat.communication": "Comunicación",
    "cat.design": "Diseño y 3D",
    "cat.dev": "Desarrollo",
    "cat.education": "Educación y ciencia",
    "cat.gaming": "Juegos",
    "cat.media": "Multimedia",
    "cat.network": "Red",
    "cat.productivity": "Productividad",
    "cat.runtimes": "Runtimes",
    "cat.security": "Seguridad",
    "cat.system": "Sistema",
    "cat.utilities": "Utilidades",
};

const es: Record<Key, string> = {...baseEs, ...healthEs, ...uiEs, ...backupEs, ...subEs, ...gamesEs};

export type Lang = "en" | "es";
export type LangSetting = Lang | "auto";

const dicts: Record<Lang, Record<Key, string>> = {en, es};
let current: Lang = "en";

/** Resolves "auto" from the browser/OS language; Spanish for any es-*. */
export function resolveLang(setting: LangSetting, navigatorLang: string): Lang {
    if (setting !== "auto") return setting;
    return navigatorLang.toLowerCase().startsWith("es") ? "es" : "en";
}

export function setLang(l: Lang): void { current = l; }
export function getLang(): Lang { return current; }

/** Translates a key, filling {placeholders}. */
export function t(key: Key, vars: Record<string, string | number> = {}): string {
    const s = dicts[current][key] ?? dicts.en[key] ?? key;
    return s.replace(/\{(\w+)\}/g, (_, k: string) => String(vars[k] ?? `{${k}}`));
}

/** Top-level category label, falling back to the raw name. */
export function categoryLabel(top: string): string {
    const key = `cat.${top}` as Key;
    return key in en ? t(key) : top;
}

/** Name of a sub-category: its translation, or the raw id when it has none yet. */
export function subLabel(top: string, sub: string): string {
    if (sub === "-") return t("sub.other");
    const key = `sub.${top}.${sub}` as Key;
    return key in en ? t(key) : sub.charAt(0).toUpperCase() + sub.slice(1);
}

export const KEYS = Object.keys(en) as Key[];
export const DICTS = dicts;

/** Looks a key up by string; null when it does not exist (finding variants are optional). */
export function tOpt(key: string, vars: Record<string, string | number> = {}): string | null {
    const s = (dicts[current] as Record<string, string>)[key] ?? (dicts.en as Record<string, string>)[key];
    if (s === undefined) return null;
    return s.replace(/\{(\w+)\}/g, (_, k: string) => String(vars[k] ?? ""));
}
