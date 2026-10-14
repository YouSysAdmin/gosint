// force-graph runs on d3-force-3d, whose API is d3-force's. Typed from
// @types/d3-force so the canvas adds its forces to the same copy
// instead of bundling d3-force a second time.
declare module 'd3-force-3d' {
  export * from 'd3-force'
}
