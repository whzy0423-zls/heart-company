import postcss from "postcss";
import valueParser from "postcss-value-parser";

const PLUGIN_NAME = "nine-xing-wechat-custom-properties";
const CUSTOM_PROPERTY_NAME = /^--[\w-]+$/;

function hasOwn(source, key) {
  return Object.prototype.hasOwnProperty.call(source, key);
}

function conditionalAncestor(declaration) {
  let parent = declaration.parent;
  while (parent && parent.type !== "root") {
    if (parent.type === "atrule") return parent;
    parent = parent.parent;
  }
  return null;
}

function collectCustomProperties(root) {
  const tokens = {};
  root.walkDecls((declaration) => {
    if (!declaration.prop.startsWith("--")) return;

    const owner = declaration.parent;
    if (
      owner?.type !== "rule" ||
      owner.selectors.some((selector) => selector.trim() !== ":root")
    ) {
      throw declaration.error(
        `WeChat style tokens must be declared in :root: ${declaration.prop}`,
        { plugin: PLUGIN_NAME },
      );
    }
    const conditional = conditionalAncestor(declaration);
    if (conditional) {
      throw declaration.error(
        `Conditional WeChat style token definition: ${declaration.prop}`,
        { plugin: PLUGIN_NAME },
      );
    }
    if (hasOwn(tokens, declaration.prop)) {
      throw declaration.error(
        `Multiple WeChat style token definitions: ${declaration.prop}`,
        { plugin: PLUGIN_NAME },
      );
    }
    tokens[declaration.prop] = declaration.value.trim();
  });
  return tokens;
}

export function extractCustomProperties(source, options = {}) {
  const root = postcss.parse(source, {
    from: options.from,
  });
  return collectCustomProperties(root);
}

function readGlobalTokens(globalTokens) {
  const loaded = typeof globalTokens === "function" ? globalTokens() : globalTokens;
  if (!loaded || typeof loaded !== "object" || Array.isArray(loaded)) return {};
  return { ...loaded };
}

function splitVarArguments(node, declaration) {
  const commaIndex = node.nodes.findIndex(
    (child) => child.type === "div" && child.value === ",",
  );
  const nameNodes = commaIndex === -1 ? node.nodes : node.nodes.slice(0, commaIndex);
  const name = valueParser.stringify(nameNodes).trim();

  if (!CUSTOM_PROPERTY_NAME.test(name)) {
    throw declaration.error(`Invalid WeChat style token reference: ${name || "<empty>"}`, {
      plugin: PLUGIN_NAME,
    });
  }

  return {
    name,
    fallback:
      commaIndex === -1
        ? undefined
        : valueParser.stringify(node.nodes.slice(commaIndex + 1)).trim(),
  };
}

function resolveNodes(nodes, tokens, declaration, resolving) {
  for (let index = 0; index < nodes.length; index += 1) {
    const node = nodes[index];
    if (node.type !== "function") continue;

    const functionName = node.value.toLowerCase();
    if (functionName === "url") continue;
    if (functionName !== "var") {
      resolveNodes(node.nodes, tokens, declaration, resolving);
      continue;
    }

    const { name, fallback } = splitVarArguments(node, declaration);
    let replacement;

    if (hasOwn(tokens, name)) {
      if (resolving.has(name)) {
        throw declaration.error(`Circular WeChat style token: ${name}`, {
          plugin: PLUGIN_NAME,
        });
      }
      const nextResolving = new Set(resolving);
      nextResolving.add(name);
      replacement = resolveValue(tokens[name], tokens, declaration, nextResolving);
    } else if (fallback !== undefined && fallback !== "") {
      replacement = resolveValue(fallback, tokens, declaration, resolving);
    } else {
      throw declaration.error(`Unresolved WeChat style token: ${name}`, {
        plugin: PLUGIN_NAME,
      });
    }

    const replacementNodes = valueParser(replacement).nodes;
    nodes.splice(index, 1, ...replacementNodes);
    index += replacementNodes.length - 1;
  }
}

function resolveValue(value, tokens, declaration, resolving = new Set()) {
  const parsed = valueParser(value);
  resolveNodes(parsed.nodes, tokens, declaration, resolving);
  return parsed.toString();
}

function mergeTokens(globalTokens, localTokens, root) {
  const tokens = { ...globalTokens };
  for (const [name, value] of Object.entries(localTokens)) {
    if (hasOwn(tokens, name) && tokens[name] !== value) {
      throw root.error(`Multiple WeChat style token definitions: ${name}`, {
        plugin: PLUGIN_NAME,
      });
    }
    tokens[name] = value;
  }
  return tokens;
}

export function createWechatCustomPropertiesPlugin({
  globalTokens = {},
  dependencyFiles = [],
} = {}) {
  return {
    postcssPlugin: PLUGIN_NAME,
    OnceExit(root, { result }) {
      for (const file of dependencyFiles) {
        result.messages.push({
          type: "dependency",
          plugin: PLUGIN_NAME,
          file,
          parent: root.source?.input?.file,
        });
      }

      const localTokens = collectCustomProperties(root);
      const tokens = mergeTokens(readGlobalTokens(globalTokens), localTokens, root);

      root.walkDecls((declaration) => {
        if (declaration.prop.startsWith("--")) {
          declaration.remove();
          return;
        }
        if (declaration.value.includes("var(")) {
          declaration.value = resolveValue(declaration.value, tokens, declaration);
        }
      });

      root.walkRules((rule) => {
        if (!rule.nodes?.length) rule.remove();
      });
    },
  };
}

createWechatCustomPropertiesPlugin.postcss = true;
