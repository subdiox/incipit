// The vendored foliate-js is plain JavaScript with no type declarations. We only
// import it for its side effect (registering the <foliate-view> custom element)
// and drive it imperatively, so a minimal ambient declaration is enough.
declare module '@/vendor/foliate-js/view.js' {
  export const makeBook: (file: unknown) => Promise<unknown>
}
