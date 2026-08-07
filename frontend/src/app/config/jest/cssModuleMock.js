const classes = new Proxy(
  {},
  {
    get: (_target, property) => (typeof property === 'string' ? property : undefined),
  },
);

export default classes;
