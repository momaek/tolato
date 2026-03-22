export async function delay(ms: number) {
  await new Promise(resolve => window.setTimeout(resolve, ms))
}
