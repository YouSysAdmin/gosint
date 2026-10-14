// seg encodes one path segment taken from a parameter: the router
// decodes a %2F in a link, so an id read from the route could otherwise
// point a request at another route. A UUID or a name passes unchanged.
export function seg(value: string): string {
  return encodeURIComponent(value)
}
