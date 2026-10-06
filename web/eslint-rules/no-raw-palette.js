const colorUtility = /^(?:bg|text|border(?:-[xytrblse])?|ring(?:-offset)?|outline|divide(?:-[xy])?|from|via|to|fill|stroke|shadow|inset-shadow|decoration|accent|caret)-/;
const rawPalette = /^(?:slate|gray|grey|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d+$|^(?:black|white)$/;
const colorProperty = /^(?:color|background(?:-color|-image)?|border(?:-[a-z]+)?|outline(?:-color)?|(?:box|text)-shadow|fill|stroke|caret-color|accent-color|text-decoration-color)$/;
const namedColors = new Set((
  'aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood ' +
  'cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray ' +
  'darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen ' +
  'darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue ' +
  'firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew ' +
  'hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan ' +
  'lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray ' +
  'lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue ' +
  'mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred ' +
  'midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid ' +
  'palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple ' +
  'rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue ' +
  'slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white ' +
  'whitesmoke yellow yellowgreen'
).split(' '));
const dynamic = '\u0000';

// Colons inside arbitrary values and selectors are not Tailwind variant separators.
function utilityPart(className) {
  let depth = 0;
  let start = 0;
  for (let index = 0; index < className.length; index += 1) {
    const character = className[index];
    if (character === '\\') {
      index += 1;
    } else if (character === '[' || character === '(') {
      depth += 1;
    } else if (character === ']' || character === ')') {
      depth -= 1;
    } else if (character === ':' && depth === 0) {
      start = index + 1;
    }
  }
  return className.slice(start).replace(/^!|!$/g, '');
}

function hasLiteralColor(value) {
  // A CSS variable's name or an image URL is not a color value.
  const colors = value
    .replace(/url\([^)]*\)/gi, '')
    .replace(/--[\w-]+/g, '');
  if (/#(?:[\da-f]{3,8})(?![\da-f])|(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(/i.test(colors)) {
    return true;
  }
  return colors.toLowerCase().split(/[^a-z-]+/).some((word) => namedColors.has(word));
}

function isRawColor(utility) {
  const prefix = utility.match(colorUtility)?.[0];
  if (prefix && rawPalette.test(utility.slice(prefix.length).split('/')[0])) return true;

  const opening = utility.indexOf('[');
  const closing = utility.lastIndexOf(']');
  if (opening < 0 || closing < opening) return false;
  const value = utility.slice(opening + 1, closing);
  if (prefix) return hasLiteralColor(value);
  if (opening !== 0) return false;
  const colon = value.indexOf(':');
  return colon < 0
    ? hasLiteralColor(value)
    : colorProperty.test(value.slice(0, colon)) && hasLiteralColor(value.slice(colon + 1));
}

function propertyName(property) {
  if (property.type !== 'Property') return undefined;
  return property.key.type === 'Identifier' && !property.computed
    ? property.key.name
    : property.key.value;
}

export default {
  meta: {
    type: 'problem',
    schema: [],
    messages: {
      rawPalette: 'Replace "{{className}}" with a semantic color utility from COLOR_GUIDE.md.',
      dynamicColor: 'Use complete semantic color classes instead of assembling "{{className}}" dynamically.',
    },
  },
  create(context) {
    const inspected = new WeakSet();

    function variableFor(identifier) {
      for (let scope = context.sourceCode.getScope(identifier); scope; scope = scope.upper) {
        const variable = scope.set.get(identifier.name);
        if (variable) return variable;
      }
      return undefined;
    }

    function unwrap(node, seen = new Set()) {
      if (!node || seen.has(node)) return undefined;
      seen.add(node);
      if (['TSAsExpression', 'TSTypeAssertion', 'TSNonNullExpression', 'TSSatisfiesExpression', 'ChainExpression'].includes(node.type)) {
        return unwrap(node.expression, seen);
      }
      if (node.type === 'Identifier') {
        const variable = variableFor(node);
        const definition = variable?.defs[0];
        if (definition?.type === 'Variable' && definition.node.id.type === 'Identifier' &&
            !variable.references.some((reference) => reference.isWrite() && !reference.init)) {
          return unwrap(definition.node.init, seen);
        }
      }
      return node;
    }

    function helperName(callee) {
      if (callee.type !== 'Identifier') return undefined;
      const definition = variableFor(callee)?.defs[0];
      if (definition?.type === 'ImportBinding') {
        const imported = definition.node.imported?.name ?? definition.node.imported?.value;
        if (['cn', 'clsx', 'cva'].includes(imported)) return imported;
        if (definition.parent.source.value === 'clsx' && definition.node.type === 'ImportDefaultSpecifier') {
          return 'clsx';
        }
      }
      return ['cn', 'clsx', 'cva'].includes(callee.name) ? callee.name : undefined;
    }

    // Preserve unknown pieces as markers, rather than inspecting unrelated expression text.
    function classText(expression, seen = new Set()) {
      const node = unwrap(expression);
      if (!node || seen.has(node)) return dynamic;
      seen.add(node);
      let result = dynamic;
      if (node.type === 'Literal' && typeof node.value === 'string') {
        result = node.value;
      } else if (node.type === 'TemplateLiteral') {
        result = node.quasis[0].value.cooked ?? node.quasis[0].value.raw;
        node.expressions.forEach((part, index) => {
          result += classText(part, seen) + (node.quasis[index + 1].value.cooked ?? node.quasis[index + 1].value.raw);
        });
      } else if (node.type === 'BinaryExpression' && node.operator === '+') {
        result = classText(node.left, seen) + classText(node.right, seen);
      }
      seen.delete(node);
      return result;
    }

    function inspectText(node) {
      for (const className of classText(node).split(/\s+/)) {
        if (!className) continue;
        const utility = utilityPart(className);
        const assembled = utility.includes(dynamic) && (
          colorUtility.test(utility) ||
          (utility.startsWith(dynamic) && utility !== dynamic) ||
          utility.startsWith('[')
        );
        const messageId = assembled ? 'dynamicColor' : isRawColor(utility) ? 'rawPalette' : undefined;
        if (messageId) {
          context.report({
            node,
            messageId,
            data: { className: className.replaceAll(dynamic, '${…}') },
          });
        }
      }
    }

    function objectProperties(expression) {
      const node = unwrap(expression);
      return node?.type === 'ObjectExpression' ? node.properties : [];
    }

    function inspectCvaOptions(expression) {
      for (const option of objectProperties(expression)) {
        if (propertyName(option) === 'variants') {
          for (const variant of objectProperties(option.value)) {
            for (const choice of objectProperties(variant.value)) {
              if (choice.type === 'Property') inspectClasses(choice.value);
            }
          }
        } else if (propertyName(option) === 'compoundVariants') {
          const compounds = unwrap(option.value);
          if (compounds?.type !== 'ArrayExpression') continue;
          for (const compound of compounds.elements) {
            for (const property of objectProperties(compound)) {
              if (['class', 'className'].includes(propertyName(property))) {
                inspectClasses(property.value);
              }
            }
          }
        }
      }
    }

    function inspectEmbedded(expression) {
      const node = unwrap(expression);
      if (!node || !classText(node).includes(dynamic)) return;
      if (node.type === 'BinaryExpression' && node.operator === '+') {
        inspectEmbedded(node.left);
        inspectEmbedded(node.right);
      } else if (node.type === 'TemplateLiteral') {
        node.expressions.forEach(inspectEmbedded);
      } else {
        inspectClasses(node);
      }
    }

    function inspectClasses(expression) {
      const node = unwrap(expression);
      if (!node || inspected.has(node)) return;
      inspected.add(node);
      switch (node.type) {
        case 'Literal':
          if (typeof node.value === 'string') inspectText(node);
          break;
        case 'TemplateLiteral':
          inspectText(node);
          node.expressions.forEach(inspectEmbedded);
          break;
        case 'BinaryExpression':
          if (node.operator === '+') {
            inspectText(node);
            inspectEmbedded(node.left);
            inspectEmbedded(node.right);
          }
          break;
        case 'ConditionalExpression':
          inspectClasses(node.consequent);
          inspectClasses(node.alternate);
          break;
        case 'LogicalExpression':
          if (node.operator !== '&&') inspectClasses(node.left);
          inspectClasses(node.right);
          break;
        case 'ArrayExpression':
          node.elements.forEach(inspectClasses);
          break;
        case 'SpreadElement':
          inspectClasses(node.argument);
          break;
        case 'ObjectExpression':
          for (const property of node.properties) {
            if (property.type === 'SpreadElement') {
              inspectClasses(property.argument);
            } else if (property.computed || property.key.type === 'Literal') {
              inspectClasses(property.key);
            }
          }
          break;
        case 'CallExpression':
          inspectCall(node);
          break;
        default:
          break;
      }
    }

    function inspectCall(node) {
      const helper = helperName(node.callee);
      if (helper === 'cva') {
        inspectClasses(node.arguments[0]);
        inspectCvaOptions(node.arguments[1]);
      } else if (helper === 'cn' || helper === 'clsx') {
        node.arguments.forEach(inspectClasses);
      }
    }

    return {
      JSXAttribute(node) {
        if (node.name.type !== 'JSXIdentifier' || node.name.name !== 'className') return;
        inspectClasses(node.value?.type === 'JSXExpressionContainer' ? node.value.expression : node.value);
      },
      CallExpression: inspectCall,
    };
  },
};
