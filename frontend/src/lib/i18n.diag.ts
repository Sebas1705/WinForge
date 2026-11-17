// Text of the Diagnostics section of Settings (the diagnostic report) and of the "Not here? Suggest this
// app" link in the Catalog. Spanish must have exactly the English keys. Nothing here comes from Go as text:
// the Go side only reports the file (as a string of JSON) and an error code.

export const diagEn = {
    "diag.section": "Diagnostics",
    "diag.sectionHint": "If something does not work, WinForge can write a small report that you attach to a bug report yourself.",
    "diag.open": "Save a diagnostic report…",
    "diag.title": "Diagnostic report",
    "diag.promise": "WinForge sends nothing anywhere. The report is only written to the file you choose, and if you report a problem you attach it yourself.",
    "diag.includes": "It contains: the WinForge version and build, the Windows version, edition and architecture, the language, whether WinForge runs as administrator, the winget version, the size of the catalog, the names, sizes and dates of WinForge's own files (never what is inside them), the end of WinForge's own log if it has one, and counts from the last scans. Your Windows user name and your user folder are removed.",
    "diag.excludes": "It never contains: the list of software installed on your PC, files or folders outside WinForge's own folder, environment variables, registry contents or the contents of your profiles.",
    "diag.previewLabel": "Exactly what will be saved",
    "diag.loading": "Preparing the report…",
    "diag.error": "The report could not be prepared: {error}",
    "diag.retry": "Try again",
    "diag.save": "Save…",
    "diag.saving": "Saving…",
    "diag.saved": "Saved to {path}. WinForge has not sent it anywhere: attach the file to your report yourself.",
    "diag.saveError": "The report could not be saved: {error}",
    "diag.err.path": "That file name is not allowed. Choose a .json or .txt file.",
    "diag.suggest.link": "Not here? Suggest this app",
    "diag.suggest.hint": "This opens your web browser on GitHub with a request already filled in for “{q}”. Only the text you typed in the search box is included, and you can change it there before sending.",
} as const;

export const diagEs: Record<keyof typeof diagEn, string> = {
    "diag.section": "Diagnóstico",
    "diag.sectionHint": "Si algo no funciona, WinForge puede escribir un pequeño informe que tú mismo adjuntas a un aviso de error.",
    "diag.open": "Guardar un informe de diagnóstico…",
    "diag.title": "Informe de diagnóstico",
    "diag.promise": "WinForge no envía nada a ningún sitio. El informe solo se escribe en el archivo que elijas y, si avisas de un problema, lo adjuntas tú mismo.",
    "diag.includes": "Contiene: la versión y la compilación de WinForge, la versión, la edición y la arquitectura de Windows, el idioma, si WinForge se ejecuta como administrador, la versión de winget, el tamaño del catálogo, los nombres, tamaños y fechas de los archivos propios de WinForge (nunca su contenido), el final del registro de WinForge si tiene uno y recuentos de los últimos análisis. Se quitan tu nombre de usuario de Windows y tu carpeta de usuario.",
    "diag.excludes": "Nunca contiene: la lista del software instalado en tu PC, archivos o carpetas fuera de la carpeta propia de WinForge, variables de entorno, contenido del registro ni el contenido de tus perfiles.",
    "diag.previewLabel": "Exactamente lo que se guardará",
    "diag.loading": "Preparando el informe…",
    "diag.error": "No se pudo preparar el informe: {error}",
    "diag.retry": "Reintentar",
    "diag.save": "Guardar…",
    "diag.saving": "Guardando…",
    "diag.saved": "Guardado en {path}. WinForge no lo ha enviado a ningún sitio: adjunta el archivo a tu aviso tú mismo.",
    "diag.saveError": "No se pudo guardar el informe: {error}",
    "diag.err.path": "Ese nombre de archivo no está permitido. Elige un archivo .json o .txt.",
    "diag.suggest.link": "¿No está aquí? Sugiere esta app",
    "diag.suggest.hint": "Esto abre tu navegador en GitHub con una petición ya rellenada para «{q}». Solo se incluye el texto que escribiste en el buscador, y allí puedes cambiarlo antes de enviar.",
};
