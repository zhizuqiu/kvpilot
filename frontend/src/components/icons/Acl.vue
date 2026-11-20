<script setup>
const props = defineProps({
    inverse: {
        type: Boolean,
        default: false,
    },
    strokeWidth: {
        type: [Number, String],
        default: 3,
    },
    strokeColor: {
        type: String,
        default: '#FFF',
    },
})
</script>

<template>
    <svg v-if="props.inverse" fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <path
            d="M24 4L40 10V22C40 32 33 40 24 44C15 40 8 32 8 22V10L24 4Z"
            fill="currentColor"
            stroke="currentColor"
            stroke-width="3"
            stroke-linejoin="round" />
        <circle cx="24" cy="21" r="4" :fill="props.strokeColor" />
        <path
            :stroke="props.strokeColor"
            :stroke-width="props.strokeWidth"
            d="M24 24V32"
            stroke-linecap="round" />
    </svg>
    <svg v-else fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <path
            d="M24 4L40 10V22C40 32 33 40 24 44C15 40 8 32 8 22V10L24 4Z"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round"
            stroke-linejoin="round" />
        <circle cx="24" cy="21" r="4" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <path
            d="M24 25V32"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round" />
    </svg>
</template>

<style lang="scss" scoped></style>
