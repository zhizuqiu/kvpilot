<script setup>
const props = defineProps({
    inverse: { type: Boolean, default: false },
    strokeWidth: { type: [Number, String], default: 3 },
    strokeColor: { type: String, default: '#FFF' },
})
</script>

<template>
    <svg v-if="props.inverse" fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <rect x="5" y="14" width="11" height="20" rx="2" fill="currentColor" />
        <rect x="32" y="14" width="11" height="20" rx="2" fill="currentColor" />
        <path d="M19 24H30" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" />
        <path d="M27 20L31 24L27 28" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
    <svg v-else fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <rect x="5" y="14" width="11" height="20" rx="2" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <rect x="32" y="14" width="11" height="20" rx="2" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <path d="M19 24H30" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" />
        <path d="M27 20L31 24L27 28" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
</template>

<style lang="scss" scoped></style>
