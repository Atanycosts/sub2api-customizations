/**
 * Common component types
 */

export interface Column {
  key: string
  label: string
  sortable?: boolean
  /** Show less frequently used fields in the mobile card's details. */
  mobileSecondary?: boolean
  class?: string
  formatter?: (value: any, row: any) => string
}
