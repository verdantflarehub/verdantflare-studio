/** Permission inspection must not let a late request overwrite browser Back. */
export async function inspectNavigation(
  signal: AbortSignal,
  location: () => string,
  inspect: () => Promise<unknown>,
): Promise<void> {
  signal.throwIfAborted()
  const initial = location()
  await inspect()
  signal.throwIfAborted()
  if (location() !== initial) throw new Error('NAVIGATION_SUPERSEDED')
}
