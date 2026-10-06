# ADR 006: Frontend Technology Stack

**Status**: Accepted
**Date**: 2025-12-18
**Feature**: 007-consent-frontend
**Partially superseded by**: [ADR 038](038-design-system-rebuilt-on-shadcn-radix.md) (UI components, animation, server-state ownership).

## Context

The consent frontend requires a modern, maintainable, and accessible web interface for end-users to manage OAuth2 delegations to AI agents. We need to select a technology stack that:

1. Provides excellent developer experience (DX)
2. Ensures type safety and reduces runtime errors
3. Delivers high performance and small bundle sizes
4. Supports accessibility (WCAG 2.1) out of the box
5. Has a mature ecosystem with good tooling
6. Aligns with team expertise and long-term maintainability

## Decision

We will use the following frontend technology stack:

**Core Framework:**
- **React 18.2+** with **TypeScript 5.3+**

**Build Tool:**
- **Vite 5.0+** (fast ESM-based bundler)

**Styling:**
- **Tailwind CSS v4.0** (utility-first CSS framework)
- **PostCSS** for CSS processing

**UI Components:**
- **Headless UI 1.7+** (accessible, unstyled components from Tailwind Labs)
- **Framer Motion 10.16+** (animations)

**HTTP Client:**
- **Axios 1.6+** (promise-based HTTP client)

**Routing:**
- **React Router DOM 6.20+** (client-side routing)

**Testing:**
- **Vitest 1.0+** (fast unit test framework, Vite-native)
- **@testing-library/react 14.1+** (component testing)
- **jsdom 27.3+** (DOM simulation)

**State Management:**
- **React Hooks** (`useState`, `useEffect`, `useReducer`)
- **Context API** (for global UI state)
- **No external state library** (no Redux, MobX, Zustand)

**Date/Time:**
- **date-fns 2.30+** (lightweight date manipulation)

## Rationale

### React 18 + TypeScript

**Why React?**
1. **Maturity**: Battle-tested, stable API, vast ecosystem
2. **Component Model**: Clear separation of concerns, reusable components
3. **Virtual DOM**: Efficient updates, good performance
4. **Hooks**: Clean state management without classes
5. **Developer Tools**: Excellent browser DevTools extension
6. **Hiring**: Large talent pool familiar with React
7. **Documentation**: Comprehensive docs, tutorials, examples
8. **Concurrent Features**: React 18 introduces concurrent rendering for better UX

**Why TypeScript?**
1. **Type Safety**: Catch errors at compile time, not runtime
2. **IDE Support**: IntelliSense, autocomplete, refactoring
3. **Documentation**: Types serve as inline documentation
4. **Refactoring**: Safe, automated refactoring
5. **Team Collaboration**: Explicit contracts between components
6. **Maintenance**: Easier to understand and modify code over time

**Alternatives Considered:**
- **Vue 3**: Good framework, but smaller ecosystem than React
- **Angular**: Too heavy, opinionated, steeper learning curve
- **Svelte**: Compile-time framework, smaller ecosystem, less mature
- **Vanilla JavaScript**: No type safety, poor DX, more bugs

**Decision**: React + TypeScript provides the best balance of maturity, ecosystem, and type safety.

### Vite 5.0

**Why Vite?**
1. **Fast HMR**: Instant hot module replacement in development
2. **ESM-Native**: Leverages native browser ES modules
3. **Fast Build**: Optimized production builds with Rollup
4. **TypeScript**: First-class TypeScript support
5. **React Support**: Official React plugin (`@vitejs/plugin-react`)
6. **Dev Experience**: Best-in-class developer experience
7. **Bundle Size**: Tree-shaking, code splitting out of the box
8. **Modern**: Designed for modern web development

**Alternatives Considered:**
- **Create React App (CRA)**: Slow, outdated, no longer maintained
- **webpack**: Slower, more complex configuration
- **Parcel**: Good, but smaller ecosystem than Vite
- **esbuild**: Very fast, but less feature-complete

**Decision**: Vite provides the best developer experience and build performance.

### Tailwind CSS v4.0

**Why Tailwind CSS?**
1. **Utility-First**: Rapid UI development with utility classes
2. **Consistency**: Design system enforced by framework
3. **Responsive**: Mobile-first responsive utilities
4. **Customization**: Fully customizable via config
5. **Bundle Size**: PurgeCSS removes unused styles (small bundles)
6. **Developer Experience**: IntelliSense, fast iteration
7. **v4.0**: New CSS-first engine, better performance
8. **No Context Switching**: Write styles inline in JSX

**Alternatives Considered:**
- **CSS Modules**: More boilerplate, slower iteration
- **Styled Components**: Runtime overhead, larger bundles
- **Emotion**: Similar to Styled Components, less adoption
- **Plain CSS/SCSS**: No design system, harder to maintain
- **Bootstrap**: Opinionated, harder to customize

**Decision**: Tailwind CSS v4.0 provides the best balance of speed, consistency, and customization.

### Headless UI

**Why Headless UI?**
1. **Accessibility**: WCAG 2.1 compliant out of the box
2. **Unstyled**: Full control over styling with Tailwind
3. **Quality**: Built by Tailwind Labs (same team)
4. **TypeScript**: Full TypeScript support
5. **React Integration**: Designed specifically for React
6. **Components**: Dialog, Menu, Switch, Listbox, Tabs, etc.
7. **Keyboard Navigation**: Proper keyboard support
8. **Screen Readers**: ARIA attributes handled correctly

**Alternatives Considered:**
- **Radix UI**: Great alternative, similar features
- **React Aria**: More low-level, steeper learning curve
- **Chakra UI**: Includes styling (conflicts with Tailwind)
- **MUI (Material-UI)**: Heavy, opinionated design
- **Ant Design**: Chinese design language, not ideal for Western UX

**Decision**: Headless UI provides the best accessibility with Tailwind integration.

### Axios

**Why Axios?**
1. **Promise-Based**: Clean async/await syntax
2. **Interceptors**: Request/response transformation and error handling
3. **Timeout**: Built-in request timeout support
4. **CSRF**: Easy to add CSRF token headers
5. **Cancellation**: Request cancellation support
6. **Browser Support**: Works in all modern browsers
7. **TypeScript**: Good TypeScript definitions

**Alternatives Considered:**
- **Fetch API**: Native, but lacks interceptors and advanced features
- **React Query**: Overkill for simple API calls, adds complexity
- **SWR**: Similar to React Query, not needed for our use case
- **GraphQL Client**: We're using REST, not GraphQL

**Decision**: Axios provides the right level of abstraction for REST APIs.

### React Router DOM 6

**Why React Router?**
1. **Industry Standard**: Most popular React routing library
2. **Declarative**: Intuitive, component-based routing
3. **Nested Routes**: Clean route nesting and layout composition
4. **Data Loading**: (v6.4+) Loader functions for data fetching
5. **TypeScript**: Good TypeScript support
6. **Code Splitting**: Lazy loading of route components
7. **History API**: Full History API support with fallback

**Alternatives Considered:**
- **TanStack Router**: New, less mature, steeper learning curve
- **Wouter**: Minimalist, but lacks features
- **Reach Router**: Deprecated, merged into React Router
- **Manual routing**: Too much boilerplate, reinventing the wheel

**Decision**: React Router is the proven, standard choice for React routing.

### Vitest

**Why Vitest?**
1. **Vite-Native**: Works seamlessly with Vite (no config mismatch)
2. **Fast**: Parallel test execution, instant watch mode
3. **Jest-Compatible**: Same API as Jest, easy migration
4. **ESM Support**: Native ESM, no transform needed
5. **TypeScript**: First-class TypeScript support
6. **Coverage**: Built-in coverage with v8 or istanbul
7. **Watch Mode**: Fast, incremental test runs

**Alternatives Considered:**
- **Jest**: Slower, requires transform, Babel config
- **Mocha**: Lower-level, more setup required
- **Jasmine**: Less popular, smaller ecosystem
- **Cypress Component Testing**: Slower, heavier

**Decision**: Vitest is the natural choice for Vite projects.

### State Management: Hooks + Context (No Redux)

**Why No External State Library?**
1. **Simplicity**: Hooks + Context is sufficient for our needs
2. **Less Boilerplate**: No actions, reducers, sagas, thunks
3. **Bundle Size**: No additional library (~40KB for Redux Toolkit)
4. **Learning Curve**: Team already knows React hooks
5. **Maintenance**: Less code, fewer abstractions
6. **Performance**: Good enough for our scale

**Our State Needs:**
- **Server State**: API data (agents, grants, services) → custom hooks with axios
- **UI State**: Loading, errors, form values → local `useState`
- **Global State**: Theme, user info → Context API (minimal)

**Alternatives Considered:**
- **Redux Toolkit**: Overkill for our simple state needs
- **MobX**: Less popular, different paradigm
- **Zustand**: Good, but unnecessary for our scale
- **Jotai/Recoil**: Atomic state, but adds complexity

**Decision**: React Hooks + Context API is sufficient and simplest.

## Decision Summary

| Category | Technology | Version | Rationale |
|----------|-----------|---------|-----------|
| Framework | React | 18.2+ | Mature, large ecosystem, hooks |
| Language | TypeScript | 5.3+ | Type safety, better DX |
| Build Tool | Vite | 5.0+ | Fast HMR, best DX |
| Styling | Tailwind CSS | 4.0 | Utility-first, fast, consistent |
| UI Components | Headless UI | 1.7+ | Accessible, unstyled |
| Animations | Framer Motion | 10.16+ | Smooth, declarative animations |
| HTTP Client | Axios | 1.6+ | Interceptors, clean API |
| Routing | React Router DOM | 6.20+ | Industry standard |
| Testing | Vitest | 1.0+ | Fast, Vite-native |
| Component Testing | Testing Library | 14.1+ | Best practices, accessibility |
| Date/Time | date-fns | 2.30+ | Lightweight, functional |
| State Management | Hooks + Context | Built-in | Simple, sufficient |

## Consequences

### Positive

1. **Type Safety**: TypeScript prevents entire classes of bugs
2. **Fast Development**: Vite HMR, Tailwind utilities, hot reload
3. **Accessibility**: Headless UI ensures WCAG compliance
4. **Performance**: Small bundles, code splitting, tree shaking
5. **Maintainability**: Clear component structure, TypeScript contracts
6. **Developer Experience**: Best-in-class tooling (Vite, TypeScript, ESLint)
7. **Hiring**: Easy to find React/TypeScript developers
8. **Future-Proof**: Modern stack, actively maintained libraries

### Negative

1. **Bundle Size**: React + dependencies ≈ 150KB gzipped (acceptable)
2. **Learning Curve**: TypeScript adds complexity for beginners
3. **Tailwind Classes**: Long className strings (can be verbose)
4. **No SSR**: Client-side only (acceptable for our use case)

### Neutral

1. **Ecosystem Lock-In**: Committed to React ecosystem (low risk, mature)
2. **Frequent Updates**: Need to keep dependencies updated (standard practice)

## Implementation Guidelines

### File Organization

```
web/src/
├── components/       # React components
│   ├── consent/      # Domain components
│   ├── layout/       # Layout components
│   └── ui/           # Generic UI components
├── hooks/            # Custom hooks
├── pages/            # Page components
├── services/         # API clients
├── types/            # TypeScript types
├── utils/            # Utility functions
└── styles/           # Global styles
```

### TypeScript Configuration

```json
{
  "compilerOptions": {
    "strict": true,
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "jsx": "react-jsx",
    "module": "ESNext",
    "moduleResolution": "node"
  }
}
```

### Tailwind Configuration

```typescript
// tailwind.config.ts
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        primary: {...},
        secondary: {...},
      },
    },
  },
};
```

### Component Patterns

**Presentational Components:**
```typescript
interface ButtonProps {
  children: React.ReactNode;
  onClick?: () => void;
  variant?: 'primary' | 'secondary';
}

export function Button({ children, onClick, variant = 'primary' }: ButtonProps) {
  const classes = variant === 'primary'
    ? 'bg-blue-600 text-white'
    : 'bg-gray-200 text-gray-800';

  return (
    <button className={`px-4 py-2 rounded ${classes}`} onClick={onClick}>
      {children}
    </button>
  );
}
```

**Custom Hooks:**
```typescript
function useAgentDelegations() {
  const [data, setData] = useState<AgentDelegation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    fetchDelegations()
      .then(setData)
      .catch(setError)
      .finally(() => setLoading(false));
  }, []);

  return { data, loading, error };
}
```

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Initial Bundle | <200KB gzipped | Code splitting helps |
| First Contentful Paint | <1.5s | On 3G connection |
| Time to Interactive | <3s | On 3G connection |
| Lighthouse Score | >90 | Performance, Accessibility, Best Practices |
| Bundle Size Growth | <10KB/month | Monitor with bundlewatch |

## Security Considerations

1. **XSS Prevention**: React escapes by default, avoid `dangerouslySetInnerHTML`
2. **Dependency Scanning**: Run `npm audit` regularly
3. **TypeScript**: Prevents type-related vulnerabilities
4. **CSP**: Work with backend to set Content Security Policy
5. **HTTPS Only**: All production traffic over HTTPS
6. **Secrets**: No secrets in frontend code (environment variables at build time)

## Migration Path

If technology needs to change:

**React → Other Framework:**
- Extract business logic to plain TypeScript modules
- Rewrite UI components in new framework
- Keep API client (Axios) and types

**Tailwind → Other CSS:**
- Extract design tokens to CSS variables
- Replace utility classes with new framework
- Keep component structure

**TypeScript → JavaScript:**
- Remove type annotations
- Keep file structure and logic
- Lose type safety (not recommended)

## Testing Strategy

1. **Unit Tests**: Component logic, utility functions
2. **Component Tests**: Rendering, user interactions (Testing Library)
3. **Integration Tests**: Component + API interactions (mocked)
4. **E2E Tests**: (Future) Full user flows (Playwright)
5. **Visual Tests**: (Future) Screenshot comparison (Chromatic)

**Coverage Target**: >80% for critical paths

## Tooling

**Development:**
- **VS Code**: Primary IDE
- **ESLint**: Code linting
- **Prettier**: Code formatting
- **TypeScript Compiler**: Type checking
- **Vite Dev Server**: Hot reload

**Build:**
- **Vite**: Bundler
- **Terser**: Minification
- **PostCSS**: CSS processing
- **TypeScript**: Compilation

**Quality:**
- **Vitest**: Unit tests
- **Testing Library**: Component tests
- **npm audit**: Security scanning
- **bundlewatch**: Bundle size monitoring

## References

- [React Documentation](https://react.dev/)
- [TypeScript Handbook](https://www.typescriptlang.org/docs/)
- [Vite Guide](https://vitejs.dev/guide/)
- [Tailwind CSS v4 Docs](https://tailwindcss.com/docs)
- [Headless UI Documentation](https://headlessui.com/)
- [Vitest Documentation](https://vitest.dev/)
- Research document: `specs/007-consent-frontend/research.md`
- Related ADR: [005-spa-serving-pattern.md](./005-spa-serving-pattern.md)
