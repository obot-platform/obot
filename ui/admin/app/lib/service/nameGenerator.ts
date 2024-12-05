import { faker } from "@faker-js/faker";

const uppercaseFirst = (word: string) =>
    word.charAt(0).toUpperCase() + word.slice(1);

const foodNouns = [
    faker.food.dish,
    faker.food.fruit,
    faker.food.ingredient,
    faker.food.meat,
    faker.food.spice,
    faker.food.vegetable,
];

const nounOptions = [
    faker.animal.type,
    faker.commerce.product,
    ...faker.helpers.arrayElements(foodNouns, 2),
    faker.hacker.noun,
    faker.person.zodiacSign,
    faker.person.jobDescriptor,
    faker.vehicle.type,
];

export function generateRandomName(): string {
    return `${faker.word.adjective()} ${faker.helpers.arrayElement(nounOptions)()}`
        .split(" ")
        .map(uppercaseFirst)
        .join(" ");
}
