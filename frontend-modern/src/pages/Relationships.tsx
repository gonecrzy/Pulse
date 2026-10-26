import { ResourceRelationshipManager } from '@/components/Infrastructure/ResourceRelationshipManager';

export function Relationships() {
  return (
    <main class="mx-auto w-full max-w-screen-2xl px-4 py-5 sm:px-6">
      <ResourceRelationshipManager inline />
    </main>
  );
}

export default Relationships;
