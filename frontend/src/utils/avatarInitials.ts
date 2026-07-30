/**
 * There is no image/upload avatar system — avatar_key is free-text (backend
 * caps it at 40 chars, but that's still far too long for the small fixed-size
 * badge every avatar renders into). This derives a short, badge-safe label
 * instead of dumping the raw stored string into the DOM.
 */
export function avatarInitials(avatarKey: string | undefined, fallback: string): string {
  const source = (avatarKey || fallback || "").trim();
  return source.slice(0, 2).toUpperCase();
}
