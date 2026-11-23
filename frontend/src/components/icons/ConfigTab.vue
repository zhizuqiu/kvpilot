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
        <circle cx="24" cy="24" r="15" fill="currentColor" />
        <circle cx="24" cy="24" r="5" :fill="props.strokeColor" />
        <path
            d="M24 4V10M24 38V44M4 24H10M38 24H44M9.9 9.9L14.1 14.1M33.9 33.9L38.1 38.1M38.1 9.9L33.9 14.1M14.1 33.9L9.9 38.1"
            :stroke="props.strokeColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round" />
    </svg>
    <svg v-else fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <circle cx="24" cy="24" r="15" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <circle cx="24" cy="24" r="5" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <path
            d="M24 2.5V9M24 39V45.5M2.5 24H9M39 24H45.5M8.8 8.8L13.4 13.4M34.6 34.6L39.2 39.2M39.2 8.8L34.6 13.4M13.4 34.6L8.8 39.2"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round" />
    </svg>
</template>

<style lang="scss" scoped></style>
