export function money(amount: number, currency: string) {
  const format = new Intl.NumberFormat("fr-FR", {
    style: "currency",
    currency,
  });
  const decimals = format.resolvedOptions().maximumFractionDigits ?? 2;
  return format.format(amount / 10 ** decimals);
}
