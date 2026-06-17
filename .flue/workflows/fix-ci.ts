import type { FlueContext } from '@flue/runtime';
import * as v from 'valibot';
import agent from '../agents/ci-fixer';

const FailuresSchema = v.object({
  job: v.string(),
  failures: v.array(v.object({
    file: v.optional(v.string()),
    line: v.optional(v.number()),
    error: v.string(),
    fix_applied: v.boolean(),
    fix_description: v.optional(v.string()),
  })),
  summary: v.string(),
});

export async function run({ init, payload }: FlueContext<{
  job?: string;
  log?: string;
}>) {
  const harness = await init(agent);
  const session = await harness.session();

  const job = payload.job ?? 'unknown';
  const log = payload.log ?? '';

  const prompt = log
    ? `CI job "${job}" failed. Here is the log output:\n\n\`\`\`\n${log}\n\`\`\`\n\nAnalyze the failure, reproduce it, fix it, and verify.`
    : `CI job "${job}" failed. Run "go test ./..." and "go build ./cmd/daemon" to find and fix any issues.`;

  const { data } = await session.prompt(prompt, {
    result: FailuresSchema,
  });

  return data;
}
