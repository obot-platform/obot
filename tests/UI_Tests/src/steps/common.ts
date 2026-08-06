import { Then } from "@wdio/cucumber-framework";
import SoftAssert from "../core/softAssert";
import { context } from "../utils/context";


Then('I setup context for assertion', async () => {
    context().softAssert = new SoftAssert();
});
