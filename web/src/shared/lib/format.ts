const dateTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
})

export function formatDateTime(value?: string | null) {
  if (!value) {
    return "N/A"
  }

  return dateTimeFormatter.format(new Date(value))
}

export function formatPercent(value?: number | null) {
  if (typeof value !== "number") {
    return "N/A"
  }

  return `${Math.round(value)}%`
}
