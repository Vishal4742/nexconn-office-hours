import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import { reloadWhenCodeIsGone } from "./staleCode";
import "./styles/tokens.css";
import "./styles/app.css";

reloadWhenCodeIsGone();

const root = document.getElementById("root");
if (!root) throw new Error("index.html is missing the #root element");

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
