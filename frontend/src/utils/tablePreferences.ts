const MIN_TABLE_PAGE_SIZE = 5

export const DEFAULT_TABLE_PAGE_SIZE = 20
export const DEFAULT_TABLE_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

const parsePageSizeForSelection = (value: unknown): number | null => {
  const size = Number(value)
  if (!Number.isInteger(size)) return null
  if (size < MIN_TABLE_PAGE_SIZE) return null
  return size
}

const normalizePageSizeToOptions = (value: number, options: number[]): number => {
  for (const option of options) {
    if (option >= value) {
      return option
    }
  }
  return options[options.length - 1]
}

export const normalizeTablePageSize = (value: unknown): number => {
  const normalized = parsePageSizeForSelection(value)
  if (normalized !== null) {
    return normalizePageSizeToOptions(normalized, DEFAULT_TABLE_PAGE_SIZE_OPTIONS)
  }
  return normalizePageSizeToOptions(DEFAULT_TABLE_PAGE_SIZE, DEFAULT_TABLE_PAGE_SIZE_OPTIONS)
}
