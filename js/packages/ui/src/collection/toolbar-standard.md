# Collection toolbar standard

This standard governs searchable, filterable and selectable collections used by Yueli websites,
applications and other Nuxt UI surfaces. It applies to tables, lists and grids; it does not replace
their data model or rendering primitive.

## Evidence

- [Nuxt UI Dashboard customers](https://github.com/nuxt-ui-templates/dashboard/blob/main/app/pages/customers.vue)
  demonstrates direct `UTable` composition, column-owned sorting, row selection and a compact toolbar.
  It is a simple one-filter example, not a specification for complex filtering.
- [Carbon data table](https://carbondesignsystem.com/components/data-table/usage/) reserves the table
  toolbar for global controls, places sorting in column headers and replaces normal controls with a
  batch action bar when rows are selected.
- [Cloudscape collection select filter](https://cloudscape.design/components/collection-select-filter/)
  recommends no more than two ordinary select filters. More properties or operators belong in a
  property filter.
- [PatternFly toolbar](https://v4-archive.patternfly.org/v4/components/toolbar/design-guidelines/)
  collapses filters behind a toggle and actions behind overflow at compact widths instead of relying
  on arbitrary wrapping.
- [Shopify Polaris index table](https://polaris-react.shopify.com/components/tables/index-table)
  treats filtering, sorting, pagination, selection and bulk actions as one collection workflow and
  permits condensed small-screen behavior when bulk selection is not essential.

## Required anatomy

1. The data surface and its controls share one visual container.
2. Default mode has one primary toolbar. From the standard Tailwind `@3xl` container size (48rem /
   768px), search, one filter trigger and a compact utility group share one row.
3. At compact component widths search owns the first row. The same filter trigger and utilities own
   a deterministic second row. Filter fields live in a popover and never expand the toolbar or wrap
   into accidental rows.
4. Selection mode replaces the default toolbar with selected count, no more than two promoted bulk
   actions, overflow for additional actions and an explicit exit. It does not append another bar.
5. Applied-filter chips use a separate optional region that exists only while filters are active.
6. Pagination and result range belong in the collection footer. Back-to-top and page actions do not.

## Filter complexity

- No useful filter: do not render an empty filter control.
- Finite properties use a flat labelled field stack inside the filter popover.
- Add sections only when the fields have a genuine semantic distinction, not merely because a count
  threshold was crossed. Multi-value conditions or operators may use a property-filter control.
- Do not change the outer toolbar anatomy based on filter count.
- Filtering always applies to the full remote collection, not only the loaded page.
- Active filters must be countable, individually removable when practical, and clearable as a group.

## Sorting and view controls

- A strict table puts sortable fields in the corresponding column headers.
- A list or grid without sortable headers may use one toolbar sort control.
- A list/table/grid view switch is a utility, not a filter.
- Column visibility is a utility and should collapse into an overflow menu when space is constrained.

## Responsive and action priority

- Responsiveness follows component width, because a collection can live in a main panel, split panel
  or embedded application shell.
- Search and the filter disclosure remain available at every supported width.
- Show at most two promoted global or bulk actions. Put the rest in an overflow menu.
- If bulk selection is essential on small screens, keep selection mode usable with compact labels or
  an overflow menu. If it is not essential, a product may deliberately enter a documented condensed
  mode; it must not disappear accidentally because controls wrapped or overflowed.

## Accessibility and internationalization

- The toolbar has a caller-owned accessible label. Visible product copy remains caller-owned and can
  come from any i18n solution.
- Search uses a real search form when explicit submission exists.
- The Nuxt UI popover trigger exposes its open state and content relationship to assistive technology.
- Selection count is textual; selection is never communicated by row color alone.
- Icon-only utilities and overflow triggers require accessible labels.

## Public Module seam

`CollectionTableToolbar` owns responsive placement, the filter popover and default/selection mode
switching. Callers own filter controls, query semantics, business actions, translated copy, table
columns, data loading and pagination. `UTable` remains the table implementation.

The Module intentionally does not expose arbitrary row-layout classes, breakpoint props or a generic
array schema for actions and filters. Those would make the Interface as complex as each product. Slots
preserve domain controls while the shared implementation pays for the difficult responsive behavior.

## Rejected patterns

- A permanent search band, permanent filter band and permanent context band.
- Unbounded `flex-wrap` as the responsive strategy.
- Direct filter selects in the outer toolbar, even when there are only one or two.
- A sticky or fixed batch bar that covers rows or pagination.
- A second table implementation or shallow `UTable` wrapper.
- Sorting both in the toolbar and the corresponding table header.
