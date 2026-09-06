/** Shared asynchronous job and event models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** identifies one supported job status value. */
enum JobStatus {
  pending,
  running,
  succeeded,
  succeeded_with_warnings,
  failed,
  outcome_unknown,
  cancelled,
}

/** identifies one supported job type value. */
enum JobType {
  `tenant.delete`,
  `repo.sync`,
  `repo.discover`,
  `asset.produce`,
  `asset.merge`,
  `asset.index`,
  `asset.ai_generate`,
  `asset.reindex`,
  `diff.run`,
  `outbox.dispatch`,
  `workspace.gc`,
  `blob.gc`,
  `retention.cleanup`,
}

/** identifies one supported job scope type value. */
enum JobScopeType {
  tenant,
  repository,
  service,
  source,
  track,
  version,
  asset,
  diff,
  system,
}

/** defines supported filters for selecting job records. */
model JobFilter {
  /** contains the ordered types associated with this job filter. */
  types?: JobType[];

  /** contains the ordered statuses associated with this job filter. */
  statuses?: JobStatus[];

  /** contains the job scope type associated with this job filter. */
  scopeType?: JobScopeType;

  /** specifies the scope id associated with this job filter. */
  scopeId?: string;
}

/** identifies one supported job trigger value. */
enum JobTrigger {
  manual,
  schedule,
  webhook,
  api,
  cli,
  system,
  retry,
  `credential-rotated`,
}

/** identifies one supported pipeline stage value. */
enum PipelineStage {
  resolve,
  discover,
  extract,
  merge,
  normalize,
  index,
}

/** represents job stage attempt data exchanged through the Meridian API. */
model JobStageAttempt {
  /** contains the pipeline stage associated with this job stage attempt. */
  stage: PipelineStage;

  /** specifies the attempt associated with this job stage attempt. */
  @minValue(1) attempt: integer;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** is the RFC 3339 UTC instant when processing began. */
  @oneOf startedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;

  /** specifies the error associated with this job stage attempt. */
  @oneOf error: ErrorResponse | null;
}

/** represents job data exchanged through the Meridian API. */
model Job {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the slug associated with this job. */
  tenantSlug: Slug;

  /** specifies the retry of job id associated with this job. */
  @oneOf retryOfJobId: Uuid | null;

  /** contains the job type associated with this job. */
  type: JobType;

  /** contains the job trigger associated with this job. */
  trigger: JobTrigger;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** specifies the stage associated with this job. */
  @oneOf stage: PipelineStage | null;

  /** contains the job scope type associated with this job. */
  scopeType: JobScopeType;

  /** specifies the scope id associated with this job. */
  scopeId: string | null;

  /** specifies the ref type associated with this job. */
  @oneOf refType: RefType | null;

  /** specifies the ref associated with this job. */
  @oneOf ref: RefName | null;

  /** Operation-specific non-secret result identifiers and counters. */
  result: Record<unknown> | null;

  /** specifies the progress associated with this job. */
  @minValue(0) @maxValue(100) progress: integer;

  /** indicates whether dirty applies to this job. */
  dirty: boolean;

  /** specifies the attempt associated with this job. */
  @minValue(0) attempt: integer;

  /** specifies the max attempts associated with this job. */
  @minValue(1) maxAttempts: integer;

  /** specifies the next attempt at associated with this job. */
  @oneOf nextAttemptAt: Timestamp | null;

  /** contains the ordered attempts associated with this job. */
  attempts: JobStageAttempt[];

  /** specifies the error associated with this job. */
  @oneOf error: ErrorResponse | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when processing began. */
  @oneOf startedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;
}

/** contains one paginated page of job records. */
model JobPage {
  ...PageInfo;

  /** contains the ordered items associated with this job page. */
  items: Job[];
}

/** represents a job log event emitted by Meridian. */
model JobLogEvent {
  /** specifies the event associated with this job log event. */
  event: "log";

  /** uniquely identifies this resource. */
  id: string;

  /** contains the timestamp associated with this job log event. */
  at: Timestamp;

  /** specifies the message associated with this job log event. */
  message: string;

  /** specifies the stage associated with this job log event. */
  @oneOf stage?: PipelineStage | null;
}

/** represents a job state event emitted by Meridian. */
model JobStateEvent {
  /** specifies the event associated with this job state event. */
  event: "state";

  /** uniquely identifies this resource. */
  id: string;

  /** contains the timestamp associated with this job state event. */
  at: Timestamp;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** specifies the progress associated with this job state event. */
  @minValue(0) @maxValue(100) progress: integer;
}

/** represents a job sse event emitted by Meridian. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "event" })
@oneOf
union JobSseEvent {
  log: JobLogEvent,
  state: JobStateEvent,
}

/** defines validated input for the corresponding Meridian API operation. */
