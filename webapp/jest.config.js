/** @type {import('ts-jest').JestConfigWithTsJest} **/
module.exports = {
    testEnvironment: 'jsdom',
    transform: {
        '^.+\\.(ts|tsx)$': 'ts-jest', // TypeScriptファイルはts-jestで処理
        '^.+\\.(js|jsx)$': 'babel-jest', // JavaScript/JSXファイルはbabel-jestで処理
    },
};
