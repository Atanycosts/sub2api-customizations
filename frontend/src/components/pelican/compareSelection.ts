export function togglePelicanComparison(selected: readonly string[], id: string): string[] {
  if (selected.includes(id)) return selected.filter((item) => item !== id)
  if (selected.length >= 2) return [...selected]
  return [...selected, id]
}
