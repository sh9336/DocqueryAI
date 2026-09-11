---
name: Grove Systems
colors:
  surface: '#0b1326'
  surface-dim: '#0b1326'
  surface-bright: '#31394d'
  surface-container-lowest: '#060e20'
  surface-container-low: '#131b2e'
  surface-container: '#171f33'
  surface-container-high: '#222a3d'
  surface-container-highest: '#2d3449'
  on-surface: '#dae2fd'
  on-surface-variant: '#c1c6d7'
  inverse-surface: '#dae2fd'
  inverse-on-surface: '#283044'
  outline: '#8b90a0'
  outline-variant: '#414755'
  surface-tint: '#adc6ff'
  primary: '#adc6ff'
  on-primary: '#002e69'
  primary-container: '#4b8eff'
  on-primary-container: '#00285c'
  inverse-primary: '#005bc1'
  secondary: '#53e16f'
  on-secondary: '#003911'
  secondary-container: '#05b046'
  on-secondary-container: '#003a11'
  tertiary: '#c0c1ff'
  on-tertiary: '#1000a9'
  tertiary-container: '#8083ff'
  on-tertiary-container: '#0d0096'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#d8e2ff'
  primary-fixed-dim: '#adc6ff'
  on-primary-fixed: '#001a41'
  on-primary-fixed-variant: '#004493'
  secondary-fixed: '#72fe88'
  secondary-fixed-dim: '#53e16f'
  on-secondary-fixed: '#002107'
  on-secondary-fixed-variant: '#00531c'
  tertiary-fixed: '#e1e0ff'
  tertiary-fixed-dim: '#c0c1ff'
  on-tertiary-fixed: '#07006c'
  on-tertiary-fixed-variant: '#2f2ebe'
  background: '#0b1326'
  on-background: '#dae2fd'
  surface-variant: '#2d3449'
typography:
  display-lg:
    fontFamily: Inter
    fontSize: 48px
    fontWeight: '700'
    lineHeight: 56px
    letterSpacing: -0.02em
  headline-lg:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '600'
    lineHeight: 40px
    letterSpacing: -0.01em
  headline-md:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
  headline-sm:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
  body-lg:
    fontFamily: Inter
    fontSize: 18px
    fontWeight: '400'
    lineHeight: 28px
  body-md:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-sm:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  label-caps:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '700'
    lineHeight: 16px
    letterSpacing: 0.05em
  code-sm:
    fontFamily: JetBrains Mono
    fontSize: 13px
    fontWeight: '450'
    lineHeight: 18px
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  base: 8px
  xs: 4px
  sm: 12px
  md: 24px
  lg: 40px
  xl: 64px
  gutter: 24px
  margin-mobile: 16px
  margin-desktop: 32px
---

## Brand & Style
This design system is engineered for high-stakes technical environments where precision, uptime, and clarity are paramount. The brand personality is authoritative and reliable, characterized by an industrial-tech aesthetic that balances professional density with modern usability.

The visual style leans into **Corporate Minimalism** with a technical edge. It utilizes a deep, dark foundation to reduce eye strain during long monitoring sessions, punctuated by high-vibrancy functional colors. The aesthetic relies on structural integrity—clear hierarchies, generous whitespace to separate complex data sets, and a refined use of depth to distinguish between background monitoring and active configuration layers.

## Colors
The palette is rooted in a "Deep Navy" foundation to provide a stable, low-fatigue environment for network operators. 

- **Primary (System Blue):** Used exclusively for primary actions, active states, and critical interactive elements.
- **Secondary (Success Green):** Reserved for "Up" status indicators, completed processes, and healthy system metrics.
- **Neutrals:** A range of slates and navies are used to build the interface hierarchy. The background uses the darkest shade, while cards and modals use progressively lighter "Surface Navy" tones to create a sense of proximity.
- **Accents:** High-contrast warnings (Amber) and errors (Red) are used sparingly to ensure immediate visual notification without cluttering the diagnostic field.

## Typography
The system utilizes **Inter** for all UI elements due to its exceptional legibility in high-density data environments. It features a tall x-height and distinct letterforms that remain clear even at small sizes.

- **Headlines:** Use Semi-Bold weights with slight negative letter-spacing for a compact, engineered feel.
- **Data Display:** For IP addresses, MAC addresses, and log files, a secondary monospaced font (**JetBrains Mono**) is implemented to ensure character alignment and readability of technical strings.
- **Labels:** Small caps with increased tracking are used for metadata headers and table columns to distinguish them from actionable content.

## Layout & Spacing
This design system employs a **12-column fluid grid** for dashboard views, allowing modules to resize based on data priority. A strict 8px spacing rhythm ensures vertical and horizontal alignment across complex components.

- **Desktop:** Employs a fixed left-hand navigation (240px) with a fluid content area. Cards typically span 3, 4, 6, or 12 columns.
- **Gaps:** Gutters are set to 24px to provide clear separation between data visualizations, preventing cognitive overload.
- **Padding:** High-density views may scale padding down to 12px (sm), while configuration modals use 32px (lg) to focus user attention.

## Elevation & Depth
Depth in this design system is primarily functional, not decorative. It uses a combination of **Tonal Layering** and **Refined Outlines** to define the stack.

1.  **Level 0 (Background):** The deepest navy (#0F172A). Used for the canvas.
2.  **Level 1 (Cards/Panels):** Raised via color (#1E293B) and a subtle 1px border (#334155).
3.  **Level 2 (Modals/Popovers):** These use a soft, extra-diffused shadow (0px 10px 30px rgba(0,0,0,0.5)) and a slightly brighter border to pull them forward from the dashboard.

Interactive elements (buttons/inputs) do not use heavy shadows; instead, they use subtle inner glows or outer "rings" on focus to indicate state.

## Shapes
The shape language is **Soft (0.25rem)**. This slight rounding provides a modern feel while maintaining the rigid, "constructed" appearance appropriate for an industrial control tool. 

- **Containers:** Dashboard cards and main containers use a 4px (0.25rem) radius.
- **Interactive Elements:** Buttons and input fields follow the same 4px radius for consistency.
- **Status Pills:** Small indicators or tags may use a fully rounded (pill) radius to distinguish them from actionable buttons.

## Components
- **Buttons:** Primary buttons are solid System Blue with white text. Secondary buttons use a "Ghost" style with a 1px slate border that illuminates on hover. All buttons include a leading icon for rapid recognition.
- **Input Fields:** Styled with a dark inset background. On focus, the border transitions to System Blue with a subtle outer glow (0px 0px 0px 2px).
- **Cards:** The foundation of the system. Each card features a "Label-Caps" header and an optional "Header Action" (e.g., Refresh, Expand).
- **Status Indicators:** "Live" status is shown via a small pulsating dot paired with the Success Green color. Error states use bold, high-contrast backgrounds to ensure they are the first thing an operator sees.
- **Data Tables:** High-density, borderless rows with subtle zebra-striping on hover. Column headers use the Label-Caps style for maximum distinction from row data.