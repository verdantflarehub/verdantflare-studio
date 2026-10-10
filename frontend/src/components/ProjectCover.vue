<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef } from "vue";
import { ProjectReader, type ProjectItem } from "../platform/project-reader";
import ProjectMedia from "./ProjectMedia.vue";
const props = defineProps<{ project: ProjectItem }>();
const root = ref<HTMLElement | null>(null),
  reader = shallowRef<ProjectReader | null>(null),
  fileID = ref<string | null>(null);
let observer: IntersectionObserver | undefined,
  disposed = false;
onMounted(() => {
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      observer?.disconnect();
      const next = new ProjectReader(
        props.project.project_id,
        props.project.head_revision_id,
      );
      reader.value = next;
      void next
        .load()
        .then(() => {
          if (!disposed)
            fileID.value =
              next.content?.cover_file_id ||
              next.content?.character?.portrait_file_id ||
              null;
        })
        .catch(() => {
          /* Opening the card presents full errors and retry. */
        });
    },
    { rootMargin: "100px" },
  );
  if (root.value) observer.observe(root.value);
});
onBeforeUnmount(() => {
  disposed = true;
  observer?.disconnect();
  reader.value?.dispose();
});
</script>

<template>
  <div ref="root" class="project-cover">
    <ProjectMedia
      v-if="reader && fileID"
      :reader="reader"
      :file-id="fileID"
      kind="image"
      :label="project.name"
      thumbnail
    />
    <span v-else aria-hidden="true">{{
      project.category === "music"
        ? "♫"
        : project.category === "video"
          ? "▷"
          : "◉"
    }}</span>
  </div>
</template>

<style scoped>
.project-cover {
  aspect-ratio: 16 / 10;
  display: grid;
  place-items: center;
  background: var(--subtle);
  border-bottom: 1px solid var(--line);
}
.project-cover > span {
  font-size: 56px;
  line-height: 1;
  color: var(--accent);
}
.project-cover > :deep(.project-media) {
  width: 100%;
  border-radius: 0;
}
</style>
