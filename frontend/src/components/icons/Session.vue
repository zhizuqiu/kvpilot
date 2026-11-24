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
        <circle cx="18" cy="17" r="7" :fill="props.strokeColor" />
        <path d="M6 39C6 31.8 11.4 26 18 26C24.6 26 30 31.8 30 39H6Z" :fill="props.strokeColor" />
        <circle cx="34" cy="20" r="5" :fill="props.strokeColor" />
        <path d="M30 38C30 32.5 32.8 29 37 29C41 29 43 32.2 43 38H30Z" :fill="props.strokeColor" />
    </svg>
    <svg v-else fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <circle cx="18" cy="17" r="7" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <path
            d="M6 39C6 31.8 11.4 26 18 26C24.6 26 30 31.8 30 39"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round" />
        <circle cx="34" cy="20" r="5" stroke="currentColor" :stroke-width="props.strokeWidth" />
        <path
            d="M30 38C30 32.5 32.8 29 37 29C41 29 43 32.2 43 38"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linecap="round" />
    </svg>
</template>

<style lang="scss" scoped></style>
