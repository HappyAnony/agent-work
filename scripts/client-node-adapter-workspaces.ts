/** Discover Node loader adapters from declared Client capabilities across all package groups. */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { z } from 'zod'

const manifestSchema = z.object({ dsh: z.object({ client: z.record(z.string(), z.unknown()).optional() }).optional() })

/**
 * Select every dynamic Client package, including API, Typert and experimental providers.
 * @param root - Repository root containing the package directories.
 * @param directories - Repository-relative workspace package directories.
 * @returns Workspaces that declare a Client capability and therefore own Node adapters.
 */
export function clientNodeAdapterWorkspaces(root: string, directories: readonly string[]): string[] {
  return directories.filter(directory => manifestSchema.parse(JSON.parse(readFileSync(join(root, directory, 'package.json'), 'utf8'))).dsh?.client !== undefined)
}
