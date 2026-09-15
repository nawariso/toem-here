// React 19 only flushes effects inside act(...) when the test environment
// advertises itself as an act environment. jest-expo does not set this flag,
// so async effects would otherwise never settle during these tests.
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
