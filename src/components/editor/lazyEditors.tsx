import { type ComponentProps, lazy, Suspense } from 'react';

const LazyAnnotationEditor = lazy(async () => import('./annotation/AnnotationEditor'));
const LazyFluxQueryEditor = lazy(async () => import('./query/flux/FluxQueryEditor'));
const LazyFSQLEditor = lazy(async () => import('./query/fsql/FSQLEditor'));

// These editors are handed to Grafana core (e.g. as `annotations.QueryEditor`),
// which renders them without providing a Suspense boundary, so each lazy editor
// has to ship with its own.
export function AnnotationEditor(props: ComponentProps<typeof LazyAnnotationEditor>) {
  return (
    <Suspense>
      <LazyAnnotationEditor {...props} />
    </Suspense>
  );
}

export function FluxQueryEditor(props: ComponentProps<typeof LazyFluxQueryEditor>) {
  return (
    <Suspense>
      <LazyFluxQueryEditor {...props} />
    </Suspense>
  );
}

export function FSQLEditor(props: ComponentProps<typeof LazyFSQLEditor>) {
  return (
    <Suspense>
      <LazyFSQLEditor {...props} />
    </Suspense>
  );
}
