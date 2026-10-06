import React from "react";
import {createRoot} from "react-dom/client";
import "@fontsource/bricolage-grotesque/latin-600.css";
import "@fontsource/bricolage-grotesque/latin-700.css";
import "@fontsource/ibm-plex-sans/latin-400.css";
import "@fontsource/ibm-plex-sans/latin-500.css";
import "@fontsource/ibm-plex-sans/latin-600.css";
import "@fontsource/ibm-plex-mono/latin-400.css";
import App from "./App";
import {ErrorBoundary} from "./components/ErrorBoundary";
import "./style.css";
import "./visual.css";

createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
        <ErrorBoundary><App/></ErrorBoundary>
    </React.StrictMode>,
);
