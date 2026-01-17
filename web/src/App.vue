<script setup lang="ts">
import GlobalProviders from "@/components/GlobalProviders.vue";
</script>

<template>
  <global-providers>
    <div id="app-root">
      <router-view />
    </div>
  </global-providers>
</template>

<style>
#app-root {
  width: 100%;
  overflow: hidden;
}
</style>
