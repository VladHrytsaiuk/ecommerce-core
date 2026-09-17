# Liluna Beauty — Design System Brief

**Market:** Spain  
**Default locale:** `es-ES`  
**Supported locales:** `es-ES`, `uk-UA`, `en`  
**Currency:** `EUR`  
**Scope:** create the design system only. Do not create a page, header, footer, catalog, mega-menu, mobile navigation, modal, search layout or any full-screen composition yet.

## Brand intent

Liluna Beauty is a Spanish online store of authentic Korean skincare and beauty products. The experience must feel warm, refined, modern, calm and trustworthy: premium without looking cold, clinical or excessively luxurious.

### Visual direction

**Contemporary editorial K-beauty**: a product-led, visually confident system with modern restraint. It should feel like a current independent beauty editorial, not a traditional cosmetics salon or a generic marketplace. Use generous but intentional whitespace, crisp hierarchy, close-up product imagery, warm off-white surfaces, bold Onest typography and selective burgundy/gold accents. The logo may carry the romantic/serif character; the UI itself remains contemporary, structured and sans-serif.

Avoid a 2010s e-commerce look: dominant full-width burgundy bars, ornamental frames, excessive thin gold rules, dated outline-icon collections, tiny low-contrast text, heavy drop shadows, rounded “app” pills everywhere, generic dashboard cards and a dense sidebar-first layout. Favor purposeful asymmetry, strong image-to-copy balance, tactile but restrained surfaces and clear functional controls.

Spanish (Spain) is the default storefront language. Ukrainian and English are full supported alternatives. Design a visible but unobtrusive language selector; every customer-facing label, navigation item, filter, validation message, empty state and commercial message must have a complete `es-ES`, `uk-UA` and `en` version. Never mix languages in a single rendered interface, use flags as the only language indicator or let the English translation become a fallback without making that behavior explicit.

The supplied logo's geometry, wordmark, illustration, proportions and clear space are immutable. Do not redraw, retype, approximate, split, rebuild or remove its background. If a transparent source asset is needed, request it instead of manipulating a PNG.

Colour is allowed to vary by context. Create documented logo variants using only approved design tokens: `Logo/Brand` (`Brand` gold), `Logo/Ink` (`MainFont`) and `Logo/Inverse` (`White`) for a dark surface. Do not use CSS filters, gradients, arbitrary replacement colours or a recoloured raster asset. If the brand palette changes later, update only the mapped token for the appropriate logo variant while preserving the exact source artwork and clear space.

The supplied screenshots are references for the existing visual DNA only: logo scale and clear space, information density, spacing rhythm and the general warm editorial character. Do not technically extract components from them or turn them into a page. Avoid a generic beauty-marketplace template, dominant burgundy bars, invented commercial claims, delivery times, shipping thresholds and trust badges.

## Approved visual tokens

| Token | Value | Use |
| --- | --- | --- |
| `MainBg` | `#FCFAF7` | Main page background |
| `White` | `#FFFFFF` | Surface / card background |
| `MainFont` | `#1A1A1A` | Main text |
| `SecondFont` | `#726C66` | Supporting text |
| `Brand` | `#A66E25` | Brand accents, decorative rules, icons and badges |
| `Accent` | `#6C2D3A` | Primary interactive elements and CTA buttons |
| `Error` | `#B84747` | Error state |
| `Success` | `#49624A` | Success state |

Accessibility: use white text on `Accent` buttons. Do not use `Brand` for normal text or small button labels until an accessible darker gold variation is approved. All interactive states must have visible keyboard focus and meet WCAG 2.2 AA.

## Typography

Use **Onest** for all UI typography. Do not introduce another typeface without approval. Normal copy uses 150–160% line-height; headings use 110–120%.

| Element | Desktop | Mobile | Weight |
| --- | --- | --- | --- |
| Promotional H1 | 56 px | 36 px | 400 |
| Page H1 | 48 px | 32 px | 700 |
| H2 | 32 px | 24 px | 700 |
| Product title | 20 px | 18 px | 600 |
| Price | 20 px | 18 px | 700 |
| Body | 16 px | 16 px | 400 |
| Menu, buttons, filters | 16 px | 14–16 px | 600 |
| Technical helper text | 12 px | 12 px | 500 |

## Layout and responsive rules

The content container is `max-width: 1610px` and centered. Below that width, it uses full available width with responsive side padding:

| Breakpoint | Content side padding | Product grid | Header height |
| --- | --- | --- | --- |
| >= 1440 px | 48 px | 3 columns | 84 px |
| 768–1439 px | 32 px | 2 columns | 84 px |
| < 768 px | 16 px | 2 columns | 60 px |

- Section spacing: 72 px desktop, 48 px mobile.
- Product-card internal spacing: 20 px desktop, 14 px mobile.
- Product-grid gap: 20 px desktop, 12 px mobile.
- Preserve enough image area for Korean beauty packaging; product cards must not crop text on product packaging or produce mismatched image ratios.

## Information architecture

Use the first seven groups as the product taxonomy in the main “Productos” mega-menu. Treat the remaining groups as commercial or utility destinations, not peer product categories.

The Spanish labels below are the `es-ES` source labels for a future navigation component. Preserve this exact taxonomy across `uk-UA` and `en`; translate labels per locale rather than changing the navigation structure. Do not design the navigation layout in this task.

### Product taxonomy

1. **Rostro**
   - Limpieza
     - Aceites y bálsamos desmaquillantes
     - Espumas, geles y cremas limpiadoras
     - Agua micelar
   - Tónicos, pads y brumas
   - Sérums y ampollas
   - Cremas y geles faciales
   - Protección solar SPF
   - Mascarillas
   - Peelings y exfoliantes
   - Contorno de ojos y labios
2. **Cabello**
   - Champús
   - Acondicionadores y mascarillas
   - Cuidado sin aclarado
   - Styling y cuero cabelludo
3. **Cuerpo**
   - Limpieza
   - Cremas y lociones
   - Manos y pies
   - Desodorantes
   - Protección solar corporal
4. **Maquillaje**
   - Bases, BB/CC creams y cushions
   - Correctores, polvos y colorete
   - Ojos
   - Labios
   - Desmaquillantes
5. **Para hombres**
   - Cuidado facial
   - Afeitado y barba
   - Cuerpo y desodorantes
   - Cabello y cuero cabelludo
   - SPF
   - Sets de regalo
6. **Sets y minis**
7. **Accesorios**

### Commercial and utility destinations

- **Regalos** — commercial gift edit, not a product taxonomy root.
- **Más vendidos** — bestseller collection.
- **Novedades** — new-arrivals collection.
- **Ofertas** — promotions and discounted products.
- **Marcas** — brand directory.
- **Encuentra tu rutina** — guided skincare finder / routine builder.

## Reference stores

Study these only for common e-commerce patterns such as product discovery, taxonomy, merchandising and checkout trust. Do not copy visual identity, layouts, copy, assets, names or components.

- https://ksisters.com.ua
- https://azi.ua
- https://makeup.com.ua/ua/
- https://www.koreanbeauty.es
- https://beaut.ua/
- https://lovalova.ua/

## Required output sequence

1. Define foundations: color roles and accessible derivatives, typography scale, spacing, radius, border, elevation, icon and responsive-grid tokens.
2. Define reusable primitives only: button, icon button, text input, select, checkbox, radio, switch, chip, badge, price and empty-state primitive. Include default, hover, focus, disabled, error and success states where applicable.
3. For decisions that are not specified - especially search, filters, borders and surface treatment - present two or three clearly named component directions instead of silently choosing one. Keep them as small component specimens, not page layouts.
4. Record every new token or deviation in **Proposed changes** with original value, proposed value, reason and affected component. Wait for approval before creating any header, footer, mega-menu, Catalog, mobile screen, modal or code.

## Deferred commerce components

Do not design, export or present a ProductCard until product structure and real product assets are supplied. The later ProductCard brief must define: image ratio and media rules, brand, translated title lengths, volume, pricing/discount rules, stock states, badges, ratings, wishlist behavior, variants and add-to-cart behavior. Do not invent these decisions from generic e-commerce conventions.

## Controlled design freedom

Treat this brief as a strong baseline, not a restriction on good design judgment.
You may adjust colors, typography scale, spacing, grid values, component details
and interaction patterns when this materially improves usability, responsive
behavior, visual hierarchy, conversion or WCAG compliance.

For every deviation, add a **Proposed changes** section with: original value,
proposed value, reason and affected screens/components. Never silently change
the logo geometry, supported locales, Spanish-default locale, EUR currency or the
approved category taxonomy; flag a change to one of those items for explicit approval first.

## System and project governance

This design system is the reusable baseline, not a frozen final product. After its initial approval, a screen project may refine an icon, component, interaction or layout for that screen. Make the refinement a clearly named local variant first; do not silently replace the global component or token.

When a local refinement is useful beyond its originating screen, add it to **Proposed system changes** with before/after specimens, rationale and affected components. Promote it to the shared design system only after approval. This applies equally to the provisional Lucide icon set: use it consistently for now, but replace or extend it later through a documented icon-system proposal rather than one-off icon substitutions.
