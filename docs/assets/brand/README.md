# VARA brand assets

The VARA mark ("the Fold") in the forms supplied with the brand artwork. The SVG files
here are kept exactly as supplied.

| File | Use |
|---|---|
| `vara-fold-icon.svg` | The full-colour mark on its own, for dark backgrounds. |
| `vara-fold-monochrome.svg` | One ink (`currentColor`), for light or dark. The Hub header uses this form. |
| `vara-fold-logo-dark.svg`, `vara-fold-logo-light.svg` | Mark with the VARA wordmark, for dark and light pages. |
| `vara-hub-logo.svg` | Mark with "VARA HUB". |

Notes for using them:

- The "VARA" lettering is outlined paths and renders the same everywhere. "HUB" and the
  taglines are live text set in Inter, falling back to Arial, so they change with the
  fonts installed.
- The taglines say what the product is meant to be, not what v0.4 does. In particular,
  "Code, Review, Build, Release" in `vara-hub-logo.svg` names features the Hub does not have
  yet (pull-request review is planned for v0.5; there is no build or release feature). Use
  the mark and "VARA HUB" without that line in the product, and bring it back when those
  features exist.
- The ivory ribbons are close to the light theme's page colour. On light pages use the
  monochrome mark or the light lockup, not the full-colour icon.
- The Hub's browser-tab icon (`frontend/public/favicon.svg`) is the full-colour icon on a
  dark rounded tile, without its drop shadow, so it reads on light and dark tabs.
