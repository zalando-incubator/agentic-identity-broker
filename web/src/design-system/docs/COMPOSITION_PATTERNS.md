# Composition Patterns

This guide shows how to effectively combine design system components to build common UI patterns and solve typical design challenges.

## Authority and example status

[DESIGN_PRINCIPLES.md](DESIGN_PRINCIPLES.md) defines the current Refined Trust Architecture direction and Principle XI process.
ADR 037 remains Proposed. These recipes retain existing examples, not a target implementation contract.
Raw palette classes and literal visual values in snippets are not new-code requirements.
Use actual semantic tokens and component APIs. Verify component props before reuse.
Self-host images and fonts. Dynamic image sources must resolve to repository-hosted assets or a local fallback.
Every component story must pass the accessibility addon in light and dark themes.
Align these recipes with the implemented target in the single cutover after ADR acceptance.

## Layout Patterns

### Centered Hero Section

```tsx
import { Container } from '@design-system/components/layout/Container';
import { Stack } from '@design-system/components/layout/Stack';

<Container size="md" className="text-center py-16">
  <Stack gap="md" align="center">
    <h1 className="text-4xl font-bold text-trust-deep">Welcome</h1>
    <p className="text-lg text-secondary">Your message here</p>
    <Button variant="primary" size="lg">
      Get Started
    </Button>
  </Stack>
</Container>;
```

### Two-Column Layout

```tsx
<Grid columns={1} gap="lg" className="md:grid-cols-2">
  <Card padding="lg">
    <h2 className="text-2xl font-bold">Left Column</h2>
    <p>Content here</p>
  </Card>
  <Card padding="lg">
    <h2 className="text-2xl font-bold">Right Column</h2>
    <p>Content here</p>
  </Card>
</Grid>
```

### Three-Column Grid

```tsx
<Grid columns={1} gap="md" className="md:grid-cols-2 lg:grid-cols-3">
  {items.map((item) => (
    <Card key={item.id} hover="lift">
      <Avatar src={item.image} />
      <h3 className="mt-4 font-semibold text-trust-deep">{item.title}</h3>
      <p className="text-sm text-secondary">{item.description}</p>
    </Card>
  ))}
</Grid>
```

### Sidebar + Content Layout

```tsx
<AppLayout
  header={<Header />}
  sidebar={<Navigation />}
  sidebarWidth="md"
  stickyHeader
>
  <Container size="lg" padding="lg">
    <h1>Main Content</h1>
  </Container>
</AppLayout>
```

## Form Patterns

### Simple Form

```tsx
<Stack gap="lg" as="form" onSubmit={handleSubmit}>
  <Stack gap="sm">
    <label htmlFor="name" className="font-medium">
      Name
    </label>
    <TextInput
      id="name"
      type="text"
      placeholder="Enter your name"
      value={name}
      onChange={(e) => setName(e.target.value)}
    />
  </Stack>

  <Stack gap="sm">
    <label htmlFor="email" className="font-medium">
      Email
    </label>
    <TextInput
      id="email"
      type="email"
      placeholder="you@example.com"
      value={email}
      onChange={(e) => setEmail(e.target.value)}
      errorMessage={emailError}
    />
  </Stack>

  <Stack direction="row" gap="md">
    <Button type="submit" variant="primary">
      Submit
    </Button>
    <Button type="button" variant="secondary" onClick={onCancel}>
      Cancel
    </Button>
  </Stack>
</Stack>
```

### Multi-Step Form

```tsx
import { PageTransition } from '@design-system/components/layout/PageTransition';

const [step, setStep] = useState(1);

<PageTransition key={step} type="slideLeft" duration={300}>
  {step === 1 && <Step1Form onNext={() => setStep(2)} />}
  {step === 2 && (
    <Step2Form onNext={() => setStep(3)} onPrev={() => setStep(1)} />
  )}
  {step === 3 && (
    <Step3Form onPrev={() => setStep(2)} onSubmit={handleSubmit} />
  )}
</PageTransition>;
```

### Form with Validation

```tsx
<Stack gap="lg">
  <TextInput
    label="Username"
    value={username}
    onChange={(e) => setUsername(e.target.value)}
    errorMessage={errors.username}
    aria-describedby={errors.username ? 'username-error' : undefined}
  />
  {errors.username && (
    <InlineError id="username-error">{errors.username}</InlineError>
  )}

  <Checkbox
    id="agree"
    label="I agree to the terms"
    checked={agree}
    onChange={(e) => setAgree(e.target.checked)}
  />
  {!agree && <Alert variant="warning">You must agree to continue</Alert>}
</Stack>
```

## Data Display Patterns

### Data Table with Sorting

```tsx
const [sortKey, setSortKey] = useState('name');
const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

<Table
  columns={[
    {
      key: 'name',
      header: 'Name',
      sortable: true,
    },
    {
      key: 'status',
      header: 'Status',
      accessor: (row) => <GrantStatusBadge status={row.status} />,
    },
    {
      key: 'date',
      header: 'Date',
      align: 'right',
      accessor: (row) => format(row.date, 'MMM d, yyyy'),
    },
  ]}
  data={data}
  sortKey={sortKey}
  sortDirection={sortDir}
  onSort={(key) => {
    if (sortKey === key) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  }}
/>;
```

### Card Grid with Hover Actions

```tsx
<Grid columns={1} gap="md" className="md:grid-cols-2 lg:grid-cols-3">
  {items.map((item) => (
    <Card
      key={item.id}
      hover="lift"
      clickable
      onClick={() => navigate(`/item/${item.id}`)}
    >
      <Stack gap="md">
        <Avatar size="lg" name={item.name} />
        <Stack gap="xs">
          <h3 className="text-lg font-semibold text-trust-deep">{item.name}</h3>
          <p className="text-sm text-secondary">{item.description}</p>
        </Stack>
        <Stack direction="row" gap="sm" className="mt-auto">
          <Badge>{item.category}</Badge>
          {item.featured && <Badge variant="success">Featured</Badge>}
        </Stack>
      </Stack>
    </Card>
  ))}
</Grid>
```

### List with Badges and Status

```tsx
<Stack gap="md" as="ul">
  {items.map((item) => (
    <li key={item.id} className="list-none">
      <Card padding="md" hover="lift">
        <Stack direction="row" gap="md" align="center" justify="space-between">
          <Stack gap="xs" flex="1">
            <h3 className="font-semibold text-trust-deep">{item.name}</h3>
            <p className="text-sm text-secondary">{item.description}</p>
          </Stack>
          <Stack direction="row" gap="sm" align="center">
            <GrantStatusBadge status={item.status} />
            <Button variant="ghost" icon={<ChevronRightIcon />} />
          </Stack>
        </Stack>
      </Card>
    </li>
  ))}
</Stack>
```

## Navigation Patterns

### Breadcrumb Navigation

```tsx
import { Breadcrumb } from '@design-system/components/navigation/Breadcrumb';

<Breadcrumb
  items={[
    { label: 'Home', href: '/' },
    { label: 'Settings', href: '/settings' },
    { label: 'Delegations', href: '/settings/delegations' },
    { label: 'Active Delegations' },
  ]}
/>;
```

### Tabs for Content Switching

```tsx
import { Tabs } from '@design-system/components/navigation/Tabs';

const [activeTab, setActiveTab] = useState('overview');

<Tabs
  tabs={[
    {
      id: 'overview',
      label: 'Overview',
      content: <OverviewPanel />,
    },
    {
      id: 'details',
      label: 'Details',
      content: <DetailsPanel />,
    },
    {
      id: 'activity',
      label: 'Activity',
      content: <ActivityPanel />,
    },
  ]}
  activeTab={activeTab}
  onChange={setActiveTab}
/>;
```

### Pagination for Large Lists

```tsx
const itemsPerPage = 10;
const [currentPage, setCurrentPage] = useState(1);
const totalPages = Math.ceil(items.length / itemsPerPage);
const paginatedItems = items.slice(
  (currentPage - 1) * itemsPerPage,
  currentPage * itemsPerPage,
);

<Stack gap="lg">
  <ItemList items={paginatedItems} />
  <Pagination
    currentPage={currentPage}
    totalPages={totalPages}
    onPageChange={setCurrentPage}
  />
</Stack>;
```

## Feedback Patterns

### Loading State

```tsx
const [loading, setLoading] = useState(true);

{
  loading ? <Skeleton count={5} /> : <ItemList items={items} />;
}
```

### Empty State

```tsx
{
  items.length === 0 ? (
    <EmptyState
      icon={<SearchIcon />}
      title="No results found"
      description="Try adjusting your filters or search query"
      action={<Button onClick={onReset}>Reset Filters</Button>}
    />
  ) : (
    <ItemList items={items} />
  );
}
```

### Error Handling

```tsx
{
  error ? (
    <Alert variant="error">
      <h3 className="font-semibold">Something went wrong</h3>
      <p>{error.message}</p>
      <Button variant="secondary" size="sm" onClick={onRetry} className="mt-4">
        Try Again
      </Button>
    </Alert>
  ) : (
    <Content />
  );
}
```

### Toast Notifications

```tsx
const [toast, setToast] = useState(null);

const showToast = (message, variant = 'success') => {
  setToast({ message, variant });
  setTimeout(() => setToast(null), 3000);
};

{
  toast && (
    <Toast
      message={toast.message}
      variant={toast.variant}
      onClose={() => setToast(null)}
    />
  );
}
```

## Permission & Consent Patterns

### Permission List with Selection

```tsx
import { ScopeList } from '@design-system/components/data-display/ScopeList';

const [selectedScopes, setSelectedScopes] = useState([]);

<ScopeList
  scopes={scopes}
  selectable={true}
  searchable={true}
  expandable={true}
  onSelectionChange={setSelectedScopes}
/>;
```

### Grant Status Display

```tsx
<Card padding="lg">
  <Stack gap="md">
    <Stack direction="row" gap="md" align="start" justify="space-between">
      <div>
        <h3 className="text-lg font-semibold text-trust-deep">{grant.name}</h3>
        <p className="text-sm text-secondary">{grant.description}</p>
      </div>
      <GrantStatusBadge status={grant.status} />
    </Stack>
    <Divider />
    <ScopeList scopes={grant.scopes} searchable={false} />
  </Stack>
</Card>
```

### Service Delegations

```tsx
import { DelegationList } from '@/components/consent/DelegationList';

<Stack gap="lg">
  <h2 className="text-2xl font-bold">Active Delegations</h2>
  <DelegationList
    delegations={delegations}
    onDelegationClick={(delegation) => navigate(`/delegation/${delegation.id}`)}
  />
</Stack>;
```

## Modal & Overlay Patterns

### Confirmation Dialog

```tsx
const [showConfirm, setShowConfirm] = useState(false);

<Modal
  isOpen={showConfirm}
  onClose={() => setShowConfirm(false)}
  title="Confirm Action"
>
  <Stack gap="lg">
    <p>Are you sure? This action cannot be undone.</p>
    <Stack direction="row" gap="md" justify="flex-end">
      <Button
        variant="secondary"
        onClick={() => setShowConfirm(false)}
      >
        Cancel
      </Button>
      <Button
        variant="danger"
        onClick={() => {
          onConfirm();
          setShowConfirm(false);
        }}
      >
        Delete
      </Button>
    </Stack>
  </Stack>
</Modal>

<Button
  variant="danger"
  onClick={() => setShowConfirm(true)}
>
  Delete Item
</Button>
```

### Dropdown Menu

```tsx
<Dropdown
  trigger={<Button icon={<MenuIcon />} />}
  items={[
    {
      id: 'edit',
      label: 'Edit',
      icon: <EditIcon />,
      onClick: () => navigate(`/edit/${id}`),
    },
    {
      id: 'duplicate',
      label: 'Duplicate',
      icon: <CopyIcon />,
      onClick: () => onDuplicate(id),
    },
    {
      id: 'delete',
      label: 'Delete',
      icon: <TrashIcon />,
      destructive: true,
      onClick: () => setShowConfirm(true),
    },
  ]}
  align="right"
/>
```

### Popover with Information

```tsx
const [showInfo, setShowInfo] = useState(false);

<Popover
  isOpen={showInfo}
  onOpenChange={setShowInfo}
  trigger={<Button icon={<InfoIcon />} variant="ghost" />}
  position="top"
>
  <Stack gap="md" padding="md" maxWidth="300px">
    <h4 className="font-semibold text-trust-deep">About This Permission</h4>
    <p className="text-sm text-secondary">
      This permission allows the application to access your email address and
      send emails on your behalf.
    </p>
    <p className="text-xs text-tertiary">Last used: Yesterday at 2:30 PM</p>
  </Stack>
</Popover>;
```

## Accordion & Collapsible Patterns

### FAQ Section

```tsx
import { Accordion } from '@design-system/components/advanced/Accordion';

<Accordion
  items={[
    {
      id: 'q1',
      title: 'What is a delegation?',
      description: 'Learn about delegations',
      content: <FAQAnswerContent id="q1" />,
    },
    {
      id: 'q2',
      title: 'How do I revoke a delegation?',
      content: <FAQAnswerContent id="q2" />,
    },
  ]}
  exclusive={true}
/>;
```

### Expandable Settings Sections

```tsx
<Accordion
  items={[
    {
      id: 'security',
      title: 'Security Settings',
      icon: <ShieldIcon />,
      content: <SecuritySettings />,
    },
    {
      id: 'privacy',
      title: 'Privacy Settings',
      icon: <LockIcon />,
      content: <PrivacySettings />,
    },
    {
      id: 'notifications',
      title: 'Notification Settings',
      icon: <BellIcon />,
      content: <NotificationSettings />,
    },
  ]}
  exclusive={false}
/>
```

## Progress & Status Patterns

### Multi-Step Wizard

```tsx
import { Progress } from '@design-system/components/advanced/Progress';

const [step, setStep] = useState(1);
const totalSteps = 4;

<Stack gap="lg">
  <Progress
    value={(step / totalSteps) * 100}
    label={`Step ${step} of ${totalSteps}`}
    showValue={true}
  />
  <WizardStep step={step} onNext={() => setStep(step + 1)} />
</Stack>;
```

### Task Completion Progress

```tsx
<Stack gap="md">
  <h3 className="font-semibold text-trust-deep">Processing Items</h3>
  {items.map((item) => (
    <Stack key={item.id} gap="xs">
      <div className="flex justify-between text-sm">
        <span>{item.name}</span>
        <span className="text-secondary">{item.progress}%</span>
      </div>
      <Progress value={item.progress} variant="success" />
    </Stack>
  ))}
</Stack>
```

## Composition Best Practices

### Do's

✅ Compose components with clear, simple prop combinations
✅ Use Stack and Grid for layout over custom divs
✅ Leverage design system tokens for consistent styling
✅ Create reusable compound patterns in application components
✅ Test accessibility of complex patterns
✅ Document custom patterns for team reuse

### Don'ts

❌ Create new components when composition works
❌ Mix custom CSS with design system components
❌ Nest too many component layers (keep reasonable depth)
❌ Ignore responsive design - compose for mobile first
❌ Forget about accessibility in complex patterns
❌ Hardcode values instead of using design tokens

## Summary

Effective component composition:

- Reduces custom code
- Maintains visual consistency
- Improves maintainability
- Speeds up development
- Ensures accessibility

Use these patterns as starting points, adapt them to your specific needs, and share new patterns back with the team.
