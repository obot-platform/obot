/**
 * @format
 *
 * global context used for sharing data from one suite to another suite
 */

const clearContext = () => {
  global.context = {
    user: {},
    envData: {},
    resources: [],
  };
};

global.context = {
  user: {},
  envData: {},
  resources: []
};

const context = () => global.context;

export {
  context,
  clearContext,
};
