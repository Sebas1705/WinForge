// Plain-language names and descriptions for the built-in profiles, in both
// interface languages. The catalog's own profile text is written for people who
// maintain the catalog ("MSVC build tools, CMake, Ninja and LLVM/Clang"); these
// say what the set is for. A test checks every built-in profile has an entry.
import type {Lang} from "./i18n";

interface Text { name: string; desc: string }

const TEXT: Record<string, { en: Text; es: Text }> = {
    "essentials": {en: {name: "Essentials", desc: "Small tools almost every PC needs."}, es: {name: "Lo básico", desc: "Pequeñas herramientas que casi todo PC necesita."}},
    "everyday": {en: {name: "Everyday", desc: "Essentials plus chat, notes and a password manager."}, es: {name: "Día a día", desc: "Lo básico más chat, notas y gestor de contraseñas."}},
    "creator": {en: {name: "Create", desc: "Record, edit and design: video, audio, images."}, es: {name: "Crear", desc: "Graba, edita y diseña: vídeo, audio e imágenes."}},
    "gaming": {en: {name: "Gaming", desc: "Game stores and chat."}, es: {name: "Juegos", desc: "Tiendas de juegos y chat."}},
    "dev-base": {en: {name: "Programming basics", desc: "Git, an editor, a modern terminal and handy tools. Every programming set builds on this."}, es: {name: "Programar: lo básico", desc: "Git, un editor, una terminal moderna y herramientas útiles. Todos los perfiles de programación parten de aquí."}},
    "dev-java": {en: {name: "Java and Kotlin", desc: "Java 21 and IntelliJ IDEA. Gradle and Maven come with each project."}, es: {name: "Java y Kotlin", desc: "Java 21 e IntelliJ IDEA. Gradle y Maven vienen con cada proyecto."}},
    "dev-android": {en: {name: "Android apps", desc: "Java tools plus Android Studio. The Android SDK downloads on first launch."}, es: {name: "Apps Android", desc: "Herramientas de Java más Android Studio. El SDK de Android se descarga al abrirlo."}},
    "dev-web": {en: {name: "Web and Node", desc: "Node.js, pnpm and an API client."}, es: {name: "Web y Node", desc: "Node.js, pnpm y un cliente de APIs."}},
    "dev-python": {en: {name: "Python", desc: "Python 3.13 and the fast uv installer."}, es: {name: "Python", desc: "Python 3.13 y el instalador rápido uv."}},
    "dev-go": {en: {name: "Go", desc: "The Go language and its tools."}, es: {name: "Go", desc: "El lenguaje Go y sus herramientas."}},
    "dev-rust": {en: {name: "Rust", desc: "Rust with the build tools it needs."}, es: {name: "Rust", desc: "Rust con las herramientas de compilación que necesita."}},
    "dev-dotnet": {en: {name: ".NET", desc: ".NET 9 and Visual Studio Community."}, es: {name: ".NET", desc: ".NET 9 y Visual Studio Community."}},
    "dev-cpp": {en: {name: "C and C++", desc: "Compilers and build tools: MSVC, CMake, Ninja, Clang."}, es: {name: "C y C++", desc: "Compiladores y herramientas: MSVC, CMake, Ninja, Clang."}},
    "dev-containers": {en: {name: "Containers and Linux", desc: "Linux inside Windows (WSL 2), Docker and Kubernetes tools."}, es: {name: "Contenedores y Linux", desc: "Linux dentro de Windows (WSL 2), Docker y Kubernetes."}},
    "dev-cloud": {en: {name: "Cloud", desc: "Command-line tools for Azure, AWS, Google Cloud and Terraform."}, es: {name: "Nube", desc: "Herramientas de línea de comandos para Azure, AWS, Google Cloud y Terraform."}},
    "dev-data": {en: {name: "Databases", desc: "A database client, PostgreSQL and a SQLite viewer."}, es: {name: "Bases de datos", desc: "Un cliente de bases de datos, PostgreSQL y un visor de SQLite."}},
    "dev-ai": {en: {name: "AI tools", desc: "Run models locally and use AI coding assistants."}, es: {name: "Herramientas de IA", desc: "Ejecuta modelos en local y usa asistentes de IA para programar."}},
    "runtimes": {en: {name: "Parts for games and old apps", desc: "The Visual C++ and .NET pieces many games and programs ask for."}, es: {name: "Piezas para juegos y apps antiguas", desc: "Las piezas de Visual C++ y .NET que piden muchos juegos y programas."}},
    "gaming-launchers": {en: {name: "All game launchers", desc: "Every major game store and launcher, on top of Gaming."}, es: {name: "Todos los lanzadores", desc: "Todas las grandes tiendas y lanzadores, además de Juegos."}},
    "retro-gaming": {en: {name: "Retro gaming", desc: "Free emulators and engines for classic games."}, es: {name: "Juegos clásicos", desc: "Emuladores y motores gratuitos para juegos clásicos."}},
    "opensource": {en: {name: "Free software", desc: "A complete desktop using only open-source programs."}, es: {name: "Software libre", desc: "Un escritorio completo solo con programas de código abierto."}},
    "privacy": {en: {name: "Privacy and security", desc: "Password manager, encryption, private browsers and messaging."}, es: {name: "Privacidad y seguridad", desc: "Gestor de contraseñas, cifrado, navegadores y mensajería privados."}},
    "writing-research": {en: {name: "Writing and study", desc: "Notes, references, LaTeX and e-books."}, es: {name: "Escribir y estudiar", desc: "Notas, referencias, LaTeX y libros electrónicos."}},
    "system-tools": {en: {name: "PC maintenance", desc: "Disk, hardware and file tools to keep your PC in shape."}, es: {name: "Mantenimiento del PC", desc: "Herramientas de discos, hardware y archivos para cuidar tu PC."}},
    "network-tools": {en: {name: "Network tools", desc: "Scanners, file transfer and private networks."}, es: {name: "Herramientas de red", desc: "Escáneres, transferencia de archivos y redes privadas."}},
    "ai-local": {en: {name: "AI on your PC", desc: "Run and chat with AI models on your own machine."}, es: {name: "IA en tu PC", desc: "Ejecuta y conversa con modelos de IA en tu propio PC."}},
    "dev-data-science": {en: {name: "Data science", desc: "Python, conda, R and notebooks."}, es: {name: "Ciencia de datos", desc: "Python, conda, R y cuadernos."}},
    "dev-games": {en: {name: "Make video games", desc: "Engines, art tools and level editors."}, es: {name: "Crear videojuegos", desc: "Motores, herramientas de arte y editores de niveles."}},
    "dev-embedded": {en: {name: "Electronics and Arduino", desc: "Arduino, circuit design and microcontroller tools."}, es: {name: "Electrónica y Arduino", desc: "Arduino, diseño de circuitos y herramientas para microcontroladores."}},
    "dev-security": {en: {name: "Security research", desc: "Debuggers, disassemblers and network scanners for authorized testing."}, es: {name: "Investigación de seguridad", desc: "Depuradores, desensambladores y escáneres para pruebas autorizadas."}},
    "dev-jvm-extras": {en: {name: "More Java tools", desc: "Other JDK builds and Kotlin and Scala tools."}, es: {name: "Más herramientas Java", desc: "Otros JDK y herramientas de Kotlin y Scala."}},
};

export const PROFILE_IDS = Object.keys(TEXT);

/** Name and description of a profile in the interface language. Profiles you made keep your own words. */
export function profileText(p: { id: string; name: string; description?: string; builtin: boolean }, lang: Lang): Text {
    const t = p.builtin ? TEXT[p.id]?.[lang] : undefined;
    return t ?? {name: p.name, desc: p.description ?? ""};
}
