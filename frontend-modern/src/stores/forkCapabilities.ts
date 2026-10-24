import { createSignal } from 'solid-js';
import {
  ForkCapabilitiesAPI,
  type ForkCapabilitySnapshot,
  type ForkFeatureID,
  type ForkProfile,
  type ForkProfileUpdate,
} from '@/api/forkCapabilities';

const [snapshot, setSnapshot] = createSignal<ForkCapabilitySnapshot | null>(null);
const [profile, setProfile] = createSignal<ForkProfile | null>(null);
const [loading, setLoading] = createSignal(false);
const [loadError, setLoadError] = createSignal<Error | null>(null);

export async function loadForkCapabilities(force = false): Promise<void> {
  if (snapshot() && !force) return;
  setLoading(true);
  try {
    const resolved = await ForkCapabilitiesAPI.getCapabilities();
    setSnapshot(resolved);
    setLoadError(null);
  } catch (error) {
    // A missing or malformed response never turns a fork feature on.
    setSnapshot(null);
    setLoadError(error instanceof Error ? error : new Error(String(error)));
  } finally {
    setLoading(false);
  }
}

export async function loadForkProfile(): Promise<void> {
  try {
    setProfile(await ForkCapabilitiesAPI.getProfile());
  } catch (error) {
    setProfile(null);
    setLoadError(error instanceof Error ? error : new Error(String(error)));
  }
}

export async function updateForkProfile(update: ForkProfileUpdate): Promise<void> {
  setLoading(true);
  try {
    const resolved = await ForkCapabilitiesAPI.updateProfile(update);
    setSnapshot(resolved);
    setProfile({
      version: resolved.profile_version,
      revision: resolved.revision,
      enabled_features: resolved.capabilities.filter((capability) => capability.available).map((capability) => capability.id),
    });
    setLoadError(null);
  } catch (error) {
    setLoadError(error instanceof Error ? error : new Error(String(error)));
    throw error;
  } finally {
    setLoading(false);
  }
}

export function hasForkCapability(id: ForkFeatureID): boolean {
  return snapshot()?.capabilities.some((capability) => capability.id === id && capability.available) ?? false;
}

export const forkCapabilitySnapshot = snapshot;
export const forkProfile = profile;
export const forkCapabilitiesLoading = loading;
export const forkCapabilitiesError = loadError;
