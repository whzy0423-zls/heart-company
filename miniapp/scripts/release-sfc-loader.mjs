import { access, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { extname } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { compileScript, parse } = require("@vue/compiler-sfc");
const UNI_APP_URL = "release-fixture:uni-app";

async function exists(url) {
  try {
    await access(fileURLToPath(url));
    return true;
  } catch {
    return false;
  }
}

export async function resolve(specifier, context, nextResolve) {
  if (specifier === "@dcloudio/uni-app") {
    return { shortCircuit: true, url: UNI_APP_URL };
  }
  if (
    specifier.startsWith(".") &&
    context.parentURL &&
    !extname(new URL(specifier, context.parentURL).pathname)
  ) {
    const jsURL = new URL(`${specifier}.js`, context.parentURL);
    if (await exists(jsURL)) return { shortCircuit: true, url: jsURL.href };
    const indexURL = new URL(`${specifier.replace(/\/$/, "")}/index.js`, context.parentURL);
    if (await exists(indexURL)) return { shortCircuit: true, url: indexURL.href };
  }
  return nextResolve(specifier, context);
}

export async function load(url, context, nextLoad) {
  if (url === UNI_APP_URL) {
    return {
      format: "module",
      shortCircuit: true,
      source: `
const register = (name) => (callback) => {
  const hooks = globalThis.__releaseUniHooks;
  if (!hooks || !Array.isArray(hooks[name])) throw new Error('release uni hook registry missing: ' + name);
  hooks[name].push(callback);
};
export const onHide = register('onHide');
export const onLoad = register('onLoad');
export const onShareAppMessage = register('onShareAppMessage');
export const onShareTimeline = register('onShareTimeline');
export const onShow = register('onShow');
export const onUnload = register('onUnload');
`,
    };
  }
  if (url.endsWith(".vue")) {
    const filename = fileURLToPath(url);
    const source = await readFile(filename, "utf8");
    const parsed = parse(source, { filename });
    if (parsed.errors.length) throw parsed.errors[0];
    if (!parsed.descriptor.scriptSetup) throw new Error(`${filename} must use <script setup>`);
    const compiled = compileScript(parsed.descriptor, {
      genDefaultAs: "__release_sfc__",
      id: `release-${Buffer.from(url).toString("hex").slice(-12)}`,
    });
    return {
      format: "module",
      shortCircuit: true,
      source: `${compiled.content}\n__release_sfc__.render = () => null;\nexport default __release_sfc__;\n`,
    };
  }
  return nextLoad(url, context);
}
