import { ResourceRelationshipManager } from '@/components/Infrastructure/ResourceRelationshipManager';
import { PageHeader } from '@/components/shared/PageHeader';

export function Relationships() {
  return (
    <main class="mx-auto w-full max-w-screen-2xl space-y-5 px-4 py-5 sm:px-6">
      <PageHeader
        title="Infrastructure relationships"
        description="Review dependencies, routes, and confirmed cross-provider identities."
      />
      <ResourceRelationshipManager inline />
    </main>
  );
}

export default Relationships;
