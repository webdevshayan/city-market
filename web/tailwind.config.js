/** Tailwind scans ONLY our own Go templates and Go source files.
 *  Output is one minified CSS file committed at site/static/styles.css
 *  and served by the web app — no CDN, no external stylesheet. */
module.exports = {
  content: [
    "site/templates/**/*.html",
    "site/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        // Brand green used across the marketplace UI.
        halal: {
          50: "#f0fdf4",
          100: "#dcfce7",
          500: "#22c55e",
          600: "#16a34a",
          700: "#15803d",
        },
      },
    },
  },
  plugins: [],
};
