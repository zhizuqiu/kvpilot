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
        <rect x="9" y="27" width="8" height="15" rx="2" fill="currentColor" />
        <rect x="20" y="19" width="8" height="23" rx="2" fill="currentColor" />
        <rect x="31" y="11" width="8" height="31" rx="2" fill="currentColor" />
        <path d="M7 43H41" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" />
    </svg>
    <svg v-else fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
        <rect
            x="9"
            y="27"
            width="8"
            height="15"
            rx="2"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linejoin="round" />
        <rect
            x="20"
            y="19"
            width="8"
            height="23"
            rx="2"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linejoin="round" />
        <rect
            x="31"
            y="11"
            width="8"
            height="31"
            rx="2"
            stroke="currentColor"
            :stroke-width="props.strokeWidth"
            stroke-linejoin="round" />
        <path d="M7 43H41" stroke="currentColor" :stroke-width="props.strokeWidth" stroke-linecap="round" />
    </svg>
</template>

<style lang="scss" scoped></style>
