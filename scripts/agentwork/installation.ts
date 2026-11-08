/** Downloadable installation guides and release notes share the verified target inventory. */
import { readFile } from 'node:fs/promises'
import type { Release } from './release.ts'

const labels = {
  en: { frontend: 'Shared frontend (already included in native packages)', 'server-linux-x64': 'Linux x64 server',
    'server-darwin-arm64': 'macOS Apple Silicon server', 'mac-arm64': 'Mac Apple Silicon app',
    'ios-simulator': 'iOS Simulator only; not installable on iPhone', 'ios-device': 'Device-specific iOS development app',
    'server-linux-arm64': 'Linux ARM64 server', 'server-darwin-x64': 'macOS Intel server', 'mac-x64': 'Mac Intel app' },
  zh: { frontend: '共享前端（原生包已内嵌）', 'server-linux-x64': 'Linux x64 服务端',
    'server-darwin-arm64': 'macOS Apple Silicon 服务端', 'mac-arm64': 'Mac Apple Silicon 应用',
    'ios-simulator': '仅限 iOS 模拟器，不能安装到 iPhone', 'ios-device': '绑定设备的 iOS 开发应用',
    'server-linux-arm64': 'Linux ARM64 服务端', 'server-darwin-x64': 'macOS Intel 服务端', 'mac-x64': 'Mac Intel 应用' },
} as const

/**
 * Render the installation tutorial with exact filenames from one release.
 * @param release - Verified release metadata.
 * @param locale - Downloadable guide language.
 * @returns Standalone Markdown, with source links pinned to the release commit.
 */
export async function installationReadme(release: Pick<Release, 'version' | 'commit' | 'artifacts'>, locale: 'en' | 'zh'): Promise<string> {
  const suffix = locale === 'zh' ? '.zh' : ''
  const template = await readFile(new URL('../../docs/cookbook/installing-agentwork-release' + suffix + '.md', import.meta.url), 'utf8')
  const body = template.replace(/^---\n[\s\S]*?\n---\n/u, '').replace(/^# .+\n/mu, '')
    .replace(/^English \| \[中文\].*\n|^\[English\].* \| 中文\n/mu, '').trim()
  const table = release.artifacts.map(row => `| ${labels[locale][row.target]} | ${row.file} |`).join('\n')
  const source = `https://github.com/HappyAnony/agent-work/tree/${release.commit}`
  return `# AgentWork ${release.version}\n\n${locale === 'zh' ? '| 用途 | 下载文件 |\n| --- | --- |' : '| Purpose | Download |\n| --- | --- |'}\n${table}\n\n${body}\n\n${locale === 'zh' ? '配套源码' : 'Matching source'}: ${source}\n`
}

/**
 * Build public release notes with direct downloads rather than opaque target identifiers.
 * @param release - Verified target inventory.
 * @param repository - Validated GitHub publication destination.
 * @returns Markdown for the GitHub release description.
 */
export function installationReleaseNotes(release: Release, repository: string): string {
  const base = `https://github.com/${repository}/releases/download/agentwork-v${release.version}/`
  const rows = release.artifacts.map(row => `| ${labels.zh[row.target]} | [${row.file}](${base}${row.file}) |`).join('\n')
  return `AgentWork ${release.version} 预发布版。客户端与服务端请使用本次配套版本。\n\n` +
    `## 下载与安装\n\n先阅读 [中文安装部署 README](${base}README.md) / [English README](${base}README.en.md)。\n\n` +
    `| 用途 | 下载 |\n| --- | --- |\n${rows}\n\n` +
    'Mac 包采用 ad-hoc 开发签名，尚无 Developer ID/公证。iOS 模拟器包不能安装到 iPhone；普通 Apple 账号的真机安装与签名步骤见 README。\n\n' +
    'Mac 服务连接凭据保存在 ~/.agent-work/client/，升级后首次连接需重新输入服务端访问令牌。\n\n' +
    `发布清单：[release.json](${base}release.json)；校验值：[SHA256SUMS](${base}SHA256SUMS)。\n`
}
