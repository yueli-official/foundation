interface MobileBottomNavItemBase {
  readonly id: string;
  readonly label: string;
  readonly icon: string;
  readonly badge?: string | number;
  /** Include badge meaning here when the visible count alone is ambiguous. */
  readonly ariaLabel?: string;
  readonly disabled?: boolean;
}

/** Products own route matching, authorization and action effects. */
export type MobileBottomNavItem = MobileBottomNavItemBase &
  (
    | { readonly to: string; readonly active?: boolean }
    | { readonly to?: never; readonly active?: never }
  );
