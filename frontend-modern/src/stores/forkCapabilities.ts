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
const [profileLoading, setProfileLoading] = createSignal(false);
const [profileError, setProfileError] = createSignal<Error | null>(null);
let sessionEpoch = 0;
let capabilityRequest = 0;
let profileRequest = 0;

const asError = (error: unknown): Error =>
  error instanceof Error ? error : new Error(String(error));

const isRevisionConflict = (error: unknown): boolean => {
  if (!error || typeof error !== 'object') return false;
  const apiError = error as { status?: number; code?: string };
  return apiError.status === 409 || apiError.code === 'stale_fork_profile';
};

const isUnauthorized = (error: unknown): boolean =>
  Boolean(error && typeof error === 'object' && (error as { status?: number }).status === 401);

export async function loadForkCapabilities(force = false): Promise<void> {
  if (snapshot() && !force) return;
  const requestEpoch = sessionEpoch;
  const requestID = ++capabilityRequest;
  setLoading(true);
  try {
    const resolved = await ForkCapabilitiesAPI.getCapabilities();
    if (requestEpoch !== sessionEpoch || requestID !== capabilityRequest) return;
    setSnapshot(resolved);
    setLoadError(null);
  } catch (error) {
    // A missing or malformed response never turns a fork feature on.
    if (requestEpoch !== sessionEpoch || requestID !== capabilityRequest) return;
    if (isUnauthorized(error)) {
      resetForkCapabilities();
      setLoadError(asError(error));
      return;
    }
    setSnapshot(null);
    setLoadError(asError(error));
  } finally {
    if (requestEpoch === sessionEpoch && requestID === capabilityRequest) setLoading(false);
  }
}

export async function loadForkProfile(): Promise<void> {
  const requestEpoch = sessionEpoch;
  const requestID = ++profileRequest;
  setProfileLoading(true);
  try {
    const resolved = await ForkCapabilitiesAPI.getProfile();
    if (requestEpoch !== sessionEpoch || requestID !== profileRequest) return;
    setProfile(resolved);
    setProfileError(null);
  } catch (error) {
    if (requestEpoch !== sessionEpoch || requestID !== profileRequest) return;
    if (isUnauthorized(error)) {
      resetForkCapabilities();
      setProfileError(asError(error));
      return;
    }
    setProfile(null);
    setProfileError(asError(error));
  } finally {
    if (requestEpoch === sessionEpoch && requestID === profileRequest) setProfileLoading(false);
  }
}

export async function updateForkProfile(update: ForkProfileUpdate): Promise<void> {
  const requestEpoch = sessionEpoch;
  setLoading(true);
  try {
    const resolved = await ForkCapabilitiesAPI.updateProfile(update);
    if (requestEpoch !== sessionEpoch) return;
    setSnapshot(resolved);
    setProfile({
      version: resolved.profile_version,
      revision: resolved.revision,
      enabled_features: resolved.capabilities
        .filter((capability) => capability.available)
        .map((capability) => capability.id),
    });
    setLoadError(null);
    setProfileError(null);
  } catch (error) {
    if (requestEpoch !== sessionEpoch) return;
    setProfileError(asError(error));
    if (isRevisionConflict(error)) {
      // The server's revision is authoritative. Refresh both read models so
      // the administrator can review the current profile before retrying.
      await Promise.all([loadForkCapabilities(true), loadForkProfile()]);
    }
    throw error;
  } finally {
    if (requestEpoch === sessionEpoch) setLoading(false);
  }
}

export function resetForkCapabilities(): void {
  sessionEpoch += 1;
  capabilityRequest += 1;
  profileRequest += 1;
  setSnapshot(null);
  setProfile(null);
  setLoading(false);
  setLoadError(null);
  setProfileLoading(false);
  setProfileError(null);
}

export function hasForkCapability(id: ForkFeatureID): boolean {
  return (
    snapshot()?.capabilities.some((capability) => capability.id === id && capability.available) ??
    false
  );
}

export const forkCapabilitySnapshot = snapshot;
export const forkProfile = profile;
export const forkCapabilitiesLoading = loading;
export const forkCapabilitiesError = loadError;
export const forkProfileLoading = profileLoading;
export const forkProfileError = profileError;
