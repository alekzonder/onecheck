import { defineConfig } from "blume";

// Every .md page under docs/ is part of the site, except internal material.
// Pages link to each other by site route ([checks](/checks)), never by .md
// path: see "Documentation site" in docs/development.md.
export default defineConfig({
  title: "onecheck",
  description:
    "Simple, scriptable health checks for a server: YAML rules, cron, exit codes, result files and an event queue.",
  logo: { image: { light: "/favicon.svg", alt: "onecheck" } },
  deployment: {
    base: "/onecheck",
    site: "https://alekzonder.github.io",
  },
  github: {
    owner: "alekzonder",
    repo: "onecheck",
    dir: "docs",
  },
  content: {
    root: ".",
    exclude: ["superpowers/**", "node_modules/**", "dist/**", "**/_*", "**/.*"],
  },
  navigation: {
    sidebar: {
      display: "group",
    },
  },
});
